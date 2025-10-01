package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

type UserResource struct{}

type UserResourceModel struct {
	ID               types.String         `tfsdk:"id"`
	Name             types.String         `tfsdk:"name"`
	AccountSeed      types.String         `tfsdk:"account_seed"`
	AllowPub         types.List           `tfsdk:"allow_pub"`
	AllowSub         types.List           `tfsdk:"allow_sub"`
	DenyPub          types.List           `tfsdk:"deny_pub"`
	DenySub          types.List           `tfsdk:"deny_sub"`
	AllowPubResponse types.Int64          `tfsdk:"allow_pub_response"`
	ResponseTTL      timetypes.GoDuration `tfsdk:"response_ttl"`
	Bearer           types.Bool           `tfsdk:"bearer"`
	Tag              types.List           `tfsdk:"tag"`
	SourceNetwork    types.List           `tfsdk:"source_network"`

	// User Limits
	MaxSubscriptions       types.Int64 `tfsdk:"max_subscriptions"`
	MaxData                types.Int64 `tfsdk:"max_data"`
	MaxPayload             types.Int64 `tfsdk:"max_payload"`
	AllowedConnectionTypes types.List  `tfsdk:"allowed_connection_types"`

	Expiry    timetypes.GoDuration `tfsdk:"expiry"`
	Start     timetypes.GoDuration `tfsdk:"start"`
	JWT       types.String         `tfsdk:"jwt"`
	Seed      types.String         `tfsdk:"seed"`
	PublicKey types.String         `tfsdk:"public_key"`
	Creds     types.String         `tfsdk:"creds"`
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a NATS JWT User",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User identifier (public key)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User name",
			},
			"account_seed": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Account seed for signing the user JWT",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"allow_pub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Publish permissions",
			},
			"allow_sub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Subscribe permissions",
			},
			"deny_pub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Deny publish permissions",
			},
			"deny_sub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Deny subscribe permissions",
			},
			"allow_pub_response": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "Allow publishing to reply subjects",
			},
			"response_ttl": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				MarkdownDescription: "Time limit for response permissions",
			},
			"bearer": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "No connect challenge required for user",
			},
			"tag": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Tags for user",
			},
			"source_network": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Source network for connection",
			},
			"expiry": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("0s"),
				MarkdownDescription: "Valid until (e.g., '720h' for 30 days, '0s' for no expiry)",
			},
			"start": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("0s"),
				MarkdownDescription: "Valid from (e.g., '72h' for 3 days, '0s' for immediately)",
			},
			"jwt": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Generated JWT token",
			},
			"seed": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "User seed (private key)",
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User public key",
			},
			"creds": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Credentials string containing JWT and seed for NATS client connection",
			},

			// User Limits
			"max_subscriptions": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of subscriptions (-1 for unlimited)",
			},
			"max_data": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of bytes (-1 for unlimited)",
			},
			"max_payload": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum message payload in bytes (-1 for unlimited)",
			},
			"allowed_connection_types": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Allowed connection types (STANDARD, WEBSOCKET, LEAFNODE, LEAFNODE_WS, MQTT, MQTT_WS, IN_PROCESS)",
			},
		},
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// No provider configuration needed
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate user key pair
	userKP, err := nkeys.CreateUser()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user keypair", err.Error())
		return
	}

	userPubKey, err := userKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get user public key", err.Error())
		return
	}

	userSeed, err := userKP.Seed()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get user seed", err.Error())
		return
	}

	// Create user claims
	userClaims := jwt.NewUserClaims(userPubKey)
	userClaims.Name = data.Name.ValueString()

	// Get account public key from seed
	accountSeedStr := data.AccountSeed.ValueString()
	if !strings.HasPrefix(accountSeedStr, "SA") {
		resp.Diagnostics.AddError(
			"Invalid account seed",
			fmt.Sprintf("Account seed must start with 'SA', got: %s...", accountSeedStr[:2]),
		)
		return
	}

	accountKP, err := nkeys.FromSeed([]byte(accountSeedStr))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore account keypair", err.Error())
		return
	}

	accountPubKey, err := accountKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account public key", err.Error())
		return
	}

	// Validate it's actually an account key
	if !strings.HasPrefix(accountPubKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid account seed",
			fmt.Sprintf("Seed does not generate an account public key (expected A*, got %s)", accountPubKey),
		)
		return
	}
	userClaims.IssuerAccount = accountPubKey

	// Handle permissions
	if !data.AllowPub.IsNull() {
		var allowPub []string
		resp.Diagnostics.Append(data.AllowPub.ElementsAs(ctx, &allowPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Pub.Allow = allowPub
	}

	if !data.AllowSub.IsNull() {
		var allowSub []string
		resp.Diagnostics.Append(data.AllowSub.ElementsAs(ctx, &allowSub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Sub.Allow = allowSub
	}

	if !data.DenyPub.IsNull() {
		var denyPub []string
		resp.Diagnostics.Append(data.DenyPub.ElementsAs(ctx, &denyPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Pub.Deny = denyPub
	}

	if !data.DenySub.IsNull() {
		var denySub []string
		resp.Diagnostics.Append(data.DenySub.ElementsAs(ctx, &denySub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Sub.Deny = denySub
	}

	// Handle response permissions
	if !data.AllowPubResponse.IsNull() {
		max := data.AllowPubResponse.ValueInt64()
		if max > 0 {
			userClaims.Permissions.Resp = &jwt.ResponsePermission{
				MaxMsgs: int(max),
			}

			if !data.ResponseTTL.IsNull() && !data.ResponseTTL.IsUnknown() {
				duration, diags := data.ResponseTTL.ValueGoDuration()
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
				userClaims.Permissions.Resp.Expires = duration
			}
		}
	}

	// Handle bearer token
	userClaims.BearerToken = data.Bearer.ValueBool()

	// Handle tags
	if !data.Tag.IsNull() {
		var tags []string
		resp.Diagnostics.Append(data.Tag.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Tags = tags
	}

	// Handle source networks
	if !data.SourceNetwork.IsNull() {
		var networks []string
		resp.Diagnostics.Append(data.SourceNetwork.ElementsAs(ctx, &networks, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Src = networks
	}

	// Handle expiry
	if !data.Expiry.IsNull() && !data.Expiry.IsUnknown() {
		duration, diags := data.Expiry.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			userClaims.Expires = time.Now().Add(duration).Unix()
		}
	}

	// Handle start time
	if !data.Start.IsNull() && !data.Start.IsUnknown() {
		duration, diags := data.Start.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			userClaims.NotBefore = time.Now().Add(duration).Unix()
		}
	}

	// Set User Limits
	if !data.MaxSubscriptions.IsNull() {
		userClaims.Limits.Subs = data.MaxSubscriptions.ValueInt64()
	}
	if !data.MaxData.IsNull() {
		userClaims.Limits.Data = data.MaxData.ValueInt64()
	}
	if !data.MaxPayload.IsNull() {
		userClaims.Limits.Payload = data.MaxPayload.ValueInt64()
	}

	// Set allowed connection types
	if !data.AllowedConnectionTypes.IsNull() {
		var connTypes []string
		resp.Diagnostics.Append(data.AllowedConnectionTypes.ElementsAs(ctx, &connTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.AllowedConnectionTypes = connTypes
	}

	// Sign the JWT with account key
	userJWT, err := userClaims.Encode(accountKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode user JWT", err.Error())
		return
	}

	// Create creds file content
	creds := fmt.Sprintf("-----BEGIN NATS USER JWT-----\n%s\n------END NATS USER JWT------\n\n-----BEGIN USER NKEY SEED-----\n%s\n------END USER NKEY SEED------\n", userJWT, string(userSeed))

	// Set computed values
	data.ID = types.StringValue(userPubKey)
	data.PublicKey = types.StringValue(userPubKey)
	data.Seed = types.StringValue(string(userSeed))
	data.JWT = types.StringValue(userJWT)
	data.Creds = types.StringValue(creds)

	tflog.Trace(ctx, "created user resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For state-only storage, nothing to read externally
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state to preserve keys
	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Recreate user claims with updated values
	userClaims := jwt.NewUserClaims(state.PublicKey.ValueString())
	userClaims.Name = data.Name.ValueString()

	// Get account public key from seed
	accountSeedStr := data.AccountSeed.ValueString()
	if !strings.HasPrefix(accountSeedStr, "SA") {
		resp.Diagnostics.AddError(
			"Invalid account seed",
			fmt.Sprintf("Account seed must start with 'SA', got: %s...", accountSeedStr[:2]),
		)
		return
	}

	accountKP, err := nkeys.FromSeed([]byte(accountSeedStr))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore account keypair", err.Error())
		return
	}

	accountPubKey, err := accountKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account public key", err.Error())
		return
	}

	// Validate it's actually an account key
	if !strings.HasPrefix(accountPubKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid account seed",
			fmt.Sprintf("Seed does not generate an account public key (expected A*, got %s)", accountPubKey),
		)
		return
	}
	userClaims.IssuerAccount = accountPubKey

	// Handle permissions (same as create)
	if !data.AllowPub.IsNull() {
		var allowPub []string
		resp.Diagnostics.Append(data.AllowPub.ElementsAs(ctx, &allowPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Pub.Allow = allowPub
	}

	if !data.AllowSub.IsNull() {
		var allowSub []string
		resp.Diagnostics.Append(data.AllowSub.ElementsAs(ctx, &allowSub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Sub.Allow = allowSub
	}

	if !data.DenyPub.IsNull() {
		var denyPub []string
		resp.Diagnostics.Append(data.DenyPub.ElementsAs(ctx, &denyPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Pub.Deny = denyPub
	}

	if !data.DenySub.IsNull() {
		var denySub []string
		resp.Diagnostics.Append(data.DenySub.ElementsAs(ctx, &denySub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Permissions.Sub.Deny = denySub
	}

	// Handle response permissions
	if !data.AllowPubResponse.IsNull() {
		max := data.AllowPubResponse.ValueInt64()
		if max > 0 {
			userClaims.Permissions.Resp = &jwt.ResponsePermission{
				MaxMsgs: int(max),
			}

			if !data.ResponseTTL.IsNull() && !data.ResponseTTL.IsUnknown() {
				duration, diags := data.ResponseTTL.ValueGoDuration()
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
				userClaims.Permissions.Resp.Expires = duration
			}
		}
	}

	// Handle bearer token
	userClaims.BearerToken = data.Bearer.ValueBool()

	// Handle tags
	if !data.Tag.IsNull() {
		var tags []string
		resp.Diagnostics.Append(data.Tag.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Tags = tags
	}

	// Handle source networks
	if !data.SourceNetwork.IsNull() {
		var networks []string
		resp.Diagnostics.Append(data.SourceNetwork.ElementsAs(ctx, &networks, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.Src = networks
	}

	// Handle expiry
	if !data.Expiry.IsNull() && !data.Expiry.IsUnknown() {
		duration, diags := data.Expiry.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			userClaims.Expires = time.Now().Add(duration).Unix()
		}
	}

	// Handle start time
	if !data.Start.IsNull() && !data.Start.IsUnknown() {
		duration, diags := data.Start.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			userClaims.NotBefore = time.Now().Add(duration).Unix()
		}
	}

	// Set User Limits
	if !data.MaxSubscriptions.IsNull() {
		userClaims.Limits.Subs = data.MaxSubscriptions.ValueInt64()
	}
	if !data.MaxData.IsNull() {
		userClaims.Limits.Data = data.MaxData.ValueInt64()
	}
	if !data.MaxPayload.IsNull() {
		userClaims.Limits.Payload = data.MaxPayload.ValueInt64()
	}

	// Set allowed connection types
	if !data.AllowedConnectionTypes.IsNull() {
		var connTypes []string
		resp.Diagnostics.Append(data.AllowedConnectionTypes.ElementsAs(ctx, &connTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		userClaims.AllowedConnectionTypes = connTypes
	}

	// Sign the JWT with account key
	userJWT, err := userClaims.Encode(accountKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode user JWT", err.Error())
		return
	}

	// Create creds file content
	creds := fmt.Sprintf("-----BEGIN NATS USER JWT-----\n%s\n------END NATS USER JWT------\n\n-----BEGIN USER NKEY SEED-----\n%s\n------END USER NKEY SEED------\n", userJWT, strings.TrimSpace(state.Seed.ValueString()))

	// Update JWT and creds while preserving keys
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Seed = state.Seed
	data.JWT = types.StringValue(userJWT)
	data.Creds = types.StringValue(creds)

	tflog.Trace(ctx, "updated user resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Nothing to clean up - all data is in state
	tflog.Trace(ctx, "deleted user resource")
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import formats:
	// - seed (just the user seed)
	// - name/seed
	// - name/seed/account_seed
	// Name can contain / encoded as // or %2F

	parts := strings.Split(req.ID, "/")

	var name string
	var userSeed string
	var accountSeed string

	// Parse from the end - seeds have predictable format
	// Check if last part is account seed (SA) or user seed (SU)
	// Format can be:
	//   - seed (just user seed)
	//   - name/seed (name and user seed)
	//   - name/seed/account_seed (all three)

	if len(parts) == 0 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be: seed, name/seed, or name/seed/account_seed",
		)
		return
	}

	lastPart := parts[len(parts)-1]

	// Check if the last part is an account seed (for the 3-part format)
	if strings.HasPrefix(lastPart, "SA") && len(parts) >= 2 {
		// Format: name/user_seed/account_seed
		accountSeed = lastPart
		userSeed = parts[len(parts)-2]

		// Validate user seed
		if !strings.HasPrefix(userSeed, "SU") {
			resp.Diagnostics.AddError(
				"Invalid user seed",
				fmt.Sprintf("Expected user seed starting with 'SU', got: %s", userSeed),
			)
			return
		}

		// Name is everything before the seeds
		if len(parts) > 2 {
			nameParts := parts[:len(parts)-2]
			name = strings.Join(nameParts, "/")
		}
	} else if strings.HasPrefix(lastPart, "SU") {
		// Format: seed or name/seed (no account seed)
		userSeed = lastPart

		// Check if we have a name
		if len(parts) > 1 {
			nameParts := parts[:len(parts)-1]
			name = strings.Join(nameParts, "/")
		}
	} else {
		resp.Diagnostics.AddError(
			"Invalid seed format",
			fmt.Sprintf("Expected user seed (SU*) or account seed (SA*), got: %s", lastPart),
		)
		return
	}

	// Decode name (handle // and %2F encodings)
	if name != "" {
		name = strings.ReplaceAll(name, "//", "\x00") // Temporary placeholder
		name = strings.ReplaceAll(name, "%2F", "/")
		name = strings.ReplaceAll(name, "\x00", "/") // Replace placeholder with /
	} else {
		name = "imported-user"
	}

	// Validate the user seed
	kp, err := nkeys.FromSeed([]byte(userSeed))
	if err != nil {
		resp.Diagnostics.AddError("Invalid user seed", fmt.Sprintf("Failed to parse seed: %v", err))
		return
	}

	publicKey, err := kp.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Invalid keypair", fmt.Sprintf("Failed to get public key: %v", err))
		return
	}

	// Validate it's a user key
	if !strings.HasPrefix(publicKey, "U") {
		resp.Diagnostics.AddError(
			"Invalid key type",
			fmt.Sprintf("Seed does not generate a user public key (expected U*, got %s)", publicKey),
		)
		return
	}

	// If no account seed provided, we can't sign JWTs, but we can still import
	if accountSeed == "" {
		resp.Diagnostics.AddWarning(
			"No account seed provided",
			"User imported without account seed. You'll need to provide account_seed to regenerate JWTs.",
		)
	} else {
		// Validate account seed
		accKP, err := nkeys.FromSeed([]byte(accountSeed))
		if err != nil {
			resp.Diagnostics.AddError("Invalid account seed", fmt.Sprintf("Failed to parse account seed: %v", err))
			return
		}
		accPubKey, err := accKP.PublicKey()
		if err == nil && !strings.HasPrefix(accPubKey, "A") {
			resp.Diagnostics.AddError(
				"Invalid account seed",
				fmt.Sprintf("Account seed does not generate an account public key (expected A*, got %s)", accPubKey),
			)
			return
		}
	}

	// Set state attributes
	resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("public_key"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("seed"), types.StringValue(userSeed))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))
	resp.State.SetAttribute(ctx, path.Root("expiry"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("start"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("allow_pub_response"), types.Int64Value(0))
	resp.State.SetAttribute(ctx, path.Root("bearer"), types.BoolValue(false))

	// Set account seed and generate JWT/creds if provided
	if accountSeed != "" {
		resp.State.SetAttribute(ctx, path.Root("account_seed"), types.StringValue(accountSeed))

		// Generate a fresh JWT
		accKP, _ := nkeys.FromSeed([]byte(accountSeed))
		accPubKey, _ := accKP.PublicKey()

		claims := jwt.NewUserClaims(publicKey)
		claims.Name = name
		claims.IssuerAccount = accPubKey

		// Encode JWT
		userJWT, err := claims.Encode(accKP)
		if err != nil {
			resp.Diagnostics.AddError("Failed to generate JWT", fmt.Sprintf("Failed to encode user JWT: %v", err))
			return
		}
		resp.State.SetAttribute(ctx, path.Root("jwt"), types.StringValue(userJWT))

		// Create creds file content
		creds := fmt.Sprintf("-----BEGIN NATS USER JWT-----\n%s\n------END NATS USER JWT------\n\n-----BEGIN USER NKEY SEED-----\n%s\n------END USER NKEY SEED------\n", userJWT, strings.TrimSpace(userSeed))
		resp.State.SetAttribute(ctx, path.Root("creds"), types.StringValue(creds))
	} else {
		resp.State.SetAttribute(ctx, path.Root("account_seed"), types.StringNull())
		resp.State.SetAttribute(ctx, path.Root("jwt"), types.StringNull())
		resp.State.SetAttribute(ctx, path.Root("creds"), types.StringNull())
	}

	// Set empty lists for permissions
	resp.State.SetAttribute(ctx, path.Root("allow_pub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("allow_sub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("deny_pub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("deny_sub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("tag"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("source_network"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("response_ttl"), timetypes.NewGoDurationNull())

	// Set null for user limit fields
	resp.State.SetAttribute(ctx, path.Root("max_subscriptions"), types.Int64Null())
	resp.State.SetAttribute(ctx, path.Root("max_data"), types.Int64Null())
	resp.State.SetAttribute(ctx, path.Root("max_payload"), types.Int64Null())
	resp.State.SetAttribute(ctx, path.Root("allowed_connection_types"), types.ListNull(types.StringType))
}
