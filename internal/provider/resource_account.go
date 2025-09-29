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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &AccountResource{}
var _ resource.ResourceWithImportState = &AccountResource{}

func NewAccountResource() resource.Resource {
	return &AccountResource{}
}

type AccountResource struct{}

type AccountResourceModel struct {
	ID               types.String         `tfsdk:"id"`
	Name             types.String         `tfsdk:"name"`
	OperatorSeed     types.String         `tfsdk:"operator_seed"`
	AllowPub         types.List           `tfsdk:"allow_pub"`
	AllowSub         types.List           `tfsdk:"allow_sub"`
	DenyPub          types.List           `tfsdk:"deny_pub"`
	DenySub          types.List           `tfsdk:"deny_sub"`
	AllowPubResponse types.Int64          `tfsdk:"allow_pub_response"`
	ResponseTTL      timetypes.GoDuration `tfsdk:"response_ttl"`
	Expiry           timetypes.GoDuration `tfsdk:"expiry"`
	Start            timetypes.GoDuration `tfsdk:"start"`
	JWT              types.String         `tfsdk:"jwt"`
	Seed             types.String         `tfsdk:"seed"`
	PublicKey        types.String         `tfsdk:"public_key"`
}

func (r *AccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (r *AccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a NATS JWT Account",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account identifier (public key)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Account name",
			},
			"operator_seed": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Operator seed for signing the account JWT",
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
			"expiry": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("0s"),
				MarkdownDescription: "Valid until (e.g., '8760h' for 1 year, '0s' for no expiry)",
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
				Sensitive:           true,
				MarkdownDescription: "Generated JWT token",
			},
			"seed": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Account seed (private key)",
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account public key",
			},
		},
	}
}


func (r *AccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate account key pair
	accountKP, err := nkeys.CreateAccount()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create account keypair", err.Error())
		return
	}

	accountPubKey, err := accountKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account public key", err.Error())
		return
	}

	accountSeed, err := accountKP.Seed()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account seed", err.Error())
		return
	}

	// Get operator public key from seed
	operatorKP, err := nkeys.FromSeed([]byte(data.OperatorSeed.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore operator keypair", err.Error())
		return
	}

	operatorPubKey, err := operatorKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get operator public key", err.Error())
		return
	}

	// Create account claims
	accountClaims := jwt.NewAccountClaims(accountPubKey)
	accountClaims.Name = data.Name.ValueString()
	accountClaims.Issuer = operatorPubKey

	// Handle permissions
	if !data.AllowPub.IsNull() {
		var allowPub []string
		resp.Diagnostics.Append(data.AllowPub.ElementsAs(ctx, &allowPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Pub.Allow = allowPub
	}

	if !data.AllowSub.IsNull() {
		var allowSub []string
		resp.Diagnostics.Append(data.AllowSub.ElementsAs(ctx, &allowSub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Sub.Allow = allowSub
	}

	if !data.DenyPub.IsNull() {
		var denyPub []string
		resp.Diagnostics.Append(data.DenyPub.ElementsAs(ctx, &denyPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Pub.Deny = denyPub
	}

	if !data.DenySub.IsNull() {
		var denySub []string
		resp.Diagnostics.Append(data.DenySub.ElementsAs(ctx, &denySub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Sub.Deny = denySub
	}

	// Handle response permissions
	if !data.AllowPubResponse.IsNull() {
		max := data.AllowPubResponse.ValueInt64()
		if max > 0 {
			accountClaims.DefaultPermissions.Resp = &jwt.ResponsePermission{
				MaxMsgs: int(max),
			}

			if !data.ResponseTTL.IsNull() && !data.ResponseTTL.IsUnknown() {
				duration, diags := data.ResponseTTL.ValueGoDuration()
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
				accountClaims.DefaultPermissions.Resp.Expires = duration
			}
		}
	}

	// Handle expiry
	if !data.Expiry.IsNull() && !data.Expiry.IsUnknown() {
		duration, diags := data.Expiry.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			accountClaims.Expires = time.Now().Add(duration).Unix()
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
			accountClaims.NotBefore = time.Now().Add(duration).Unix()
		}
	}

	// Sign the JWT with operator key (already have operatorKP from above)
	accountJWT, err := accountClaims.Encode(operatorKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode account JWT", err.Error())
		return
	}

	// Set computed values
	data.ID = types.StringValue(accountPubKey)
	data.PublicKey = types.StringValue(accountPubKey)
	data.Seed = types.StringValue(string(accountSeed))
	data.JWT = types.StringValue(accountJWT)

	tflog.Trace(ctx, "created account resource")

	// TODO: Implement automatic push to NATS in future version
	// Requires proper NATS server configuration with resolver that handles
	// $SYS.REQ.CLAIMS.UPDATE or $SYS.REQ.ACCOUNT.<key>.CLAIMS.UPDATE requests

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For state-only storage, nothing to read externally
}

func (r *AccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state to preserve keys
	var state AccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get operator public key from seed
	operatorKP, err := nkeys.FromSeed([]byte(data.OperatorSeed.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore operator keypair", err.Error())
		return
	}

	operatorPubKey, err := operatorKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get operator public key", err.Error())
		return
	}

	// Recreate account claims with updated values
	accountClaims := jwt.NewAccountClaims(state.PublicKey.ValueString())
	accountClaims.Name = data.Name.ValueString()
	accountClaims.Issuer = operatorPubKey

	// Handle permissions (same as create)
	if !data.AllowPub.IsNull() {
		var allowPub []string
		resp.Diagnostics.Append(data.AllowPub.ElementsAs(ctx, &allowPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Pub.Allow = allowPub
	}

	if !data.AllowSub.IsNull() {
		var allowSub []string
		resp.Diagnostics.Append(data.AllowSub.ElementsAs(ctx, &allowSub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Sub.Allow = allowSub
	}

	if !data.DenyPub.IsNull() {
		var denyPub []string
		resp.Diagnostics.Append(data.DenyPub.ElementsAs(ctx, &denyPub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Pub.Deny = denyPub
	}

	if !data.DenySub.IsNull() {
		var denySub []string
		resp.Diagnostics.Append(data.DenySub.ElementsAs(ctx, &denySub, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		accountClaims.DefaultPermissions.Sub.Deny = denySub
	}

	// Handle response permissions
	if !data.AllowPubResponse.IsNull() {
		max := data.AllowPubResponse.ValueInt64()
		if max > 0 {
			accountClaims.DefaultPermissions.Resp = &jwt.ResponsePermission{
				MaxMsgs: int(max),
			}

			if !data.ResponseTTL.IsNull() && !data.ResponseTTL.IsUnknown() {
				duration, diags := data.ResponseTTL.ValueGoDuration()
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
				accountClaims.DefaultPermissions.Resp.Expires = duration
			}
		}
	}

	// Handle expiry
	if !data.Expiry.IsNull() && !data.Expiry.IsUnknown() {
		duration, diags := data.Expiry.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			accountClaims.Expires = time.Now().Add(duration).Unix()
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
			accountClaims.NotBefore = time.Now().Add(duration).Unix()
		}
	}

	// Sign the JWT with operator key (already have operatorKP from above)
	accountJWT, err := accountClaims.Encode(operatorKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode account JWT", err.Error())
		return
	}

	// Update JWT while preserving keys
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Seed = state.Seed
	data.JWT = types.StringValue(accountJWT)

	tflog.Trace(ctx, "updated account resource")

	// TODO: Implement automatic push to NATS in future version

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Nothing to clean up - all data is in state
	tflog.Trace(ctx, "deleted account resource")
}

func (r *AccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import formats:
	// - seed (just the account seed)
	// - name/seed
	// - name/seed/operator_seed
	// Name can contain / encoded as // or %2F

	parts := strings.Split(req.ID, "/")

	var name string
	var accountSeed string
	var operatorSeed string

	// Parse from the end - seeds have predictable format
	// Check if last part is operator seed (SO) or account seed (SA)
	// Format can be:
	//   - seed (just account seed)
	//   - name/seed (name and account seed)
	//   - name/seed/operator_seed (all three)

	if len(parts) == 0 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be: seed, name/seed, or name/seed/operator_seed",
		)
		return
	}

	lastPart := parts[len(parts)-1]

	// Check if the last part is an operator seed (for the 3-part format)
	if strings.HasPrefix(lastPart, "SO") && len(parts) >= 2 {
		// Format: name/account_seed/operator_seed
		operatorSeed = lastPart
		accountSeed = parts[len(parts)-2]

		// Validate account seed
		if !strings.HasPrefix(accountSeed, "SA") {
			resp.Diagnostics.AddError(
				"Invalid account seed",
				fmt.Sprintf("Expected account seed starting with 'SA', got: %s", accountSeed),
			)
			return
		}

		// Name is everything before the seeds
		if len(parts) > 2 {
			nameParts := parts[:len(parts)-2]
			name = strings.Join(nameParts, "/")
		}
	} else if strings.HasPrefix(lastPart, "SA") {
		// Format: seed or name/seed (no operator seed)
		accountSeed = lastPart

		// Check if we have a name
		if len(parts) > 1 {
			nameParts := parts[:len(parts)-1]
			name = strings.Join(nameParts, "/")
		}
	} else {
		resp.Diagnostics.AddError(
			"Invalid seed format",
			fmt.Sprintf("Expected account seed (SA*) or operator seed (SO*), got: %s", lastPart),
		)
		return
	}

	// Decode name (handle // and %2F encodings)
	if name != "" {
		name = strings.ReplaceAll(name, "//", "\x00") // Temporary placeholder
		name = strings.ReplaceAll(name, "%2F", "/")
		name = strings.ReplaceAll(name, "\x00", "/")   // Replace placeholder with /
	} else {
		name = "imported-account"
	}

	// Validate the account seed
	kp, err := nkeys.FromSeed([]byte(accountSeed))
	if err != nil {
		resp.Diagnostics.AddError("Invalid account seed", fmt.Sprintf("Failed to parse seed: %v", err))
		return
	}

	publicKey, err := kp.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Invalid keypair", fmt.Sprintf("Failed to get public key: %v", err))
		return
	}

	// Validate it's an account key
	if !strings.HasPrefix(publicKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid key type",
			fmt.Sprintf("Seed does not generate an account public key (expected A*, got %s)", publicKey),
		)
		return
	}

	// If no operator seed provided, we can't sign JWTs, but we can still import
	if operatorSeed == "" {
		resp.Diagnostics.AddWarning(
			"No operator seed provided",
			"Account imported without operator seed. You'll need to provide operator_seed to regenerate JWTs.",
		)
	} else {
		// Validate operator seed
		opKP, err := nkeys.FromSeed([]byte(operatorSeed))
		if err != nil {
			resp.Diagnostics.AddError("Invalid operator seed", fmt.Sprintf("Failed to parse operator seed: %v", err))
			return
		}
		opPubKey, err := opKP.PublicKey()
		if err == nil && !strings.HasPrefix(opPubKey, "O") {
			resp.Diagnostics.AddError(
				"Invalid operator seed",
				fmt.Sprintf("Operator seed does not generate an operator public key (expected O*, got %s)", opPubKey),
			)
			return
		}
	}

	// Set state attributes
	resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("public_key"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("seed"), types.StringValue(accountSeed))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))
	resp.State.SetAttribute(ctx, path.Root("expiry"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("start"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("allow_pub_response"), types.Int64Value(0))

	// Set operator seed if provided
	if operatorSeed != "" {
		resp.State.SetAttribute(ctx, path.Root("operator_seed"), types.StringValue(operatorSeed))

		// Generate a fresh JWT
		opKP, _ := nkeys.FromSeed([]byte(operatorSeed))
		opPubKey, _ := opKP.PublicKey()

		claims := jwt.NewAccountClaims(publicKey)
		claims.Name = name
		claims.Issuer = opPubKey

		// Encode JWT
		accountJWT, err := claims.Encode(opKP)
		if err != nil {
			resp.Diagnostics.AddError("Failed to generate JWT", fmt.Sprintf("Failed to encode account JWT: %v", err))
			return
		}
		resp.State.SetAttribute(ctx, path.Root("jwt"), types.StringValue(accountJWT))
	} else {
		resp.State.SetAttribute(ctx, path.Root("operator_seed"), types.StringNull())
		resp.State.SetAttribute(ctx, path.Root("jwt"), types.StringNull())
	}

	// Set empty lists for permissions
	resp.State.SetAttribute(ctx, path.Root("allow_pub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("allow_sub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("deny_pub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("deny_sub"), types.ListNull(types.StringType))
	resp.State.SetAttribute(ctx, path.Root("response_ttl"), timetypes.NewGoDurationNull())
}

// pushAccountToNATS pushes an account JWT to the configured NATS server