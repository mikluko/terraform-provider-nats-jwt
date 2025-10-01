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
	Subject          types.String         `tfsdk:"subject"`
	IssuerSeed       types.String         `tfsdk:"issuer_seed"`
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
	PublicKey types.String         `tfsdk:"public_key"`
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
			"subject": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User public key (subject of the JWT)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"issuer_seed": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Account seed for signing the user JWT (issuer)",
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
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User public key (same as subject)",
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

	// Get user public key (subject)
	userPubKey := data.Subject.ValueString()
	if !strings.HasPrefix(userPubKey, "U") {
		resp.Diagnostics.AddError(
			"Invalid user public key",
			fmt.Sprintf("User public key must start with 'U', got: %s", userPubKey[:1]),
		)
		return
	}

	// Get account seed (issuer) for signing
	accountSeedStr := data.IssuerSeed.ValueString()
	if !strings.HasPrefix(accountSeedStr, "SA") {
		resp.Diagnostics.AddError(
			"Invalid issuer seed",
			fmt.Sprintf("Account seed must start with 'SA', got: %s", accountSeedStr[:2]),
		)
		return
	}

	accountKP, err := nkeys.FromSeed([]byte(accountSeedStr))
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse issuer seed", err.Error())
		return
	}

	// Verify the seed produces an account key
	accountPubKey, err := accountKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get public key from issuer seed", err.Error())
		return
	}
	if !strings.HasPrefix(accountPubKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid issuer seed",
			fmt.Sprintf("Issuer seed does not generate an account public key (expected A*, got %s)", accountPubKey),
		)
		return
	}

	// Create user claims
	userClaims := jwt.NewUserClaims(userPubKey)
	userClaims.Name = data.Name.ValueString()
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

	// Set computed values
	data.ID = types.StringValue(userPubKey)
	data.PublicKey = types.StringValue(userPubKey)
	data.JWT = types.StringValue(userJWT)

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

	// Get current state to preserve immutable fields
	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get user public key and account seed from state (immutable)
	userPubKey := state.Subject.ValueString()
	accountSeedStr := state.IssuerSeed.ValueString()

	accountKP, err := nkeys.FromSeed([]byte(accountSeedStr))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore account keypair", err.Error())
		return
	}

	// Get account public key
	accountPubKey, err := accountKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account public key", err.Error())
		return
	}

	// Create user claims with updated values
	userClaims := jwt.NewUserClaims(userPubKey)
	userClaims.Name = data.Name.ValueString()
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

	// Update JWT while preserving immutable fields
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Subject = state.Subject
	data.IssuerSeed = state.IssuerSeed
	data.JWT = types.StringValue(userJWT)

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
	// Import format: name/subject/issuer_seed
	// Name can contain / encoded as // or %2F

	parts := strings.Split(req.ID, "/")

	if len(parts) < 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be: name/subject/issuer_seed",
		)
		return
	}

	// Last part is the issuer seed (account seed)
	issuerSeed := parts[len(parts)-1]
	if !strings.HasPrefix(issuerSeed, "SA") {
		resp.Diagnostics.AddError(
			"Invalid issuer seed",
			fmt.Sprintf("Expected account seed starting with 'SA', got: %s", issuerSeed),
		)
		return
	}

	// Second to last part is the subject (user public key)
	subject := parts[len(parts)-2]
	if !strings.HasPrefix(subject, "U") {
		resp.Diagnostics.AddError(
			"Invalid subject",
			fmt.Sprintf("Expected user public key starting with 'U', got: %s", subject),
		)
		return
	}

	// Everything before is the name
	nameParts := parts[:len(parts)-2]
	name := strings.Join(nameParts, "/")

	// Decode name (handle // and %2F encodings)
	name = strings.ReplaceAll(name, "//", "\x00") // Temporary placeholder
	name = strings.ReplaceAll(name, "%2F", "/")
	name = strings.ReplaceAll(name, "\x00", "/") // Replace placeholder with /

	// Validate the issuer seed
	accKP, err := nkeys.FromSeed([]byte(issuerSeed))
	if err != nil {
		resp.Diagnostics.AddError("Invalid issuer seed", fmt.Sprintf("Failed to parse seed: %v", err))
		return
	}

	accPubKey, err := accKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Invalid keypair", fmt.Sprintf("Failed to get public key: %v", err))
		return
	}

	// Validate it's an account key
	if !strings.HasPrefix(accPubKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid key type",
			fmt.Sprintf("Issuer seed does not generate an account public key (expected A*, got %s)", accPubKey),
		)
		return
	}

	// Generate a fresh JWT
	claims := jwt.NewUserClaims(subject)
	claims.Name = name
	claims.IssuerAccount = accPubKey

	// Encode JWT
	userJWT, err := claims.Encode(accKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate JWT", fmt.Sprintf("Failed to encode user JWT: %v", err))
		return
	}

	// Set state attributes
	resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(subject))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))
	resp.State.SetAttribute(ctx, path.Root("subject"), types.StringValue(subject))
	resp.State.SetAttribute(ctx, path.Root("issuer_seed"), types.StringValue(issuerSeed))
	resp.State.SetAttribute(ctx, path.Root("expiry"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("start"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("allow_pub_response"), types.Int64Value(0))
	resp.State.SetAttribute(ctx, path.Root("bearer"), types.BoolValue(false))
	resp.State.SetAttribute(ctx, path.Root("jwt"), types.StringValue(userJWT))
	resp.State.SetAttribute(ctx, path.Root("public_key"), types.StringValue(subject))

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
