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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &OperatorResource{}
var _ resource.ResourceWithImportState = &OperatorResource{}

func NewOperatorResource() resource.Resource {
	return &OperatorResource{}
}

type OperatorResource struct{}

type OperatorResourceModel struct {
	ID                  types.String         `tfsdk:"id"`
	Name                types.String         `tfsdk:"name"`
	GenerateSigningKey  types.Bool           `tfsdk:"generate_signing_key"`
	CreateSystemAccount types.Bool           `tfsdk:"create_system_account"`
	SystemAccountName   types.String         `tfsdk:"system_account_name"`
	Expiry              timetypes.GoDuration `tfsdk:"expiry"`
	Start               timetypes.GoDuration `tfsdk:"start"`
	JWT                 types.String         `tfsdk:"jwt"`
	Seed                types.String         `tfsdk:"seed"`
	PublicKey           types.String         `tfsdk:"public_key"`
	SigningKeySeed      types.String         `tfsdk:"signing_key_seed"`
	SigningKey          types.String         `tfsdk:"signing_key"`
	SystemAccount       types.String         `tfsdk:"system_account"`
	SystemAccountJWT    types.String         `tfsdk:"system_account_jwt"`
	SystemAccountSeed   types.String         `tfsdk:"system_account_seed"`
}

func (r *OperatorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_operator"
}

func (r *OperatorResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a NATS JWT Operator",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operator identifier (public key)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Operator name",
			},
			"generate_signing_key": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Generate a signing key with the operator",
			},
			"create_system_account": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Create and manage a system account for this operator",
			},
			"system_account_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("SYS"),
				MarkdownDescription: "Name for the system account (defaults to 'SYS')",
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
				MarkdownDescription: "Operator seed (private key)",
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operator public key",
			},
			"signing_key_seed": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Signing key seed (if generated)",
			},
			"signing_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Signing key public key (if generated)",
			},
			"system_account": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "System account public key (if created)",
			},
			"system_account_jwt": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "System account JWT (if created)",
			},
			"system_account_seed": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "System account seed (if created)",
			},
		},
	}
}

func (r *OperatorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// No provider configuration needed
}

func (r *OperatorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OperatorResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate operator key pair
	operatorKP, err := nkeys.CreateOperator()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create operator keypair", err.Error())
		return
	}

	operatorPubKey, err := operatorKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get operator public key", err.Error())
		return
	}

	operatorSeed, err := operatorKP.Seed()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get operator seed", err.Error())
		return
	}

	// Create operator claims
	operatorClaims := jwt.NewOperatorClaims(operatorPubKey)
	operatorClaims.Name = data.Name.ValueString()

	// Handle expiry
	if !data.Expiry.IsNull() && !data.Expiry.IsUnknown() {
		duration, diags := data.Expiry.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			operatorClaims.Expires = time.Now().Add(duration).Unix()
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
			operatorClaims.NotBefore = time.Now().Add(duration).Unix()
		}
	}

	// Generate signing key if requested
	var signingKeySeed []byte
	var signingKeyPub string
	if data.GenerateSigningKey.ValueBool() {
		signingKP, err := nkeys.CreateOperator()
		if err != nil {
			resp.Diagnostics.AddError("Failed to create signing keypair", err.Error())
			return
		}

		signingKeyPub, err = signingKP.PublicKey()
		if err != nil {
			resp.Diagnostics.AddError("Failed to get signing public key", err.Error())
			return
		}

		signingKeySeed, err = signingKP.Seed()
		if err != nil {
			resp.Diagnostics.AddError("Failed to get signing seed", err.Error())
			return
		}

		operatorClaims.SigningKeys.Add(signingKeyPub)
	}

	// Create system account if requested
	var systemAccountJWT, systemAccountPubKey string
	var systemAccountSeed []byte
	if data.CreateSystemAccount.ValueBool() {
		// Generate system account key pair
		sysAccountKP, err := nkeys.CreateAccount()
		if err != nil {
			resp.Diagnostics.AddError("Failed to create system account keypair", err.Error())
			return
		}

		systemAccountPubKey, err = sysAccountKP.PublicKey()
		if err != nil {
			resp.Diagnostics.AddError("Failed to get system account public key", err.Error())
			return
		}

		systemAccountSeed, err = sysAccountKP.Seed()
		if err != nil {
			resp.Diagnostics.AddError("Failed to get system account seed", err.Error())
			return
		}

		// Set system account in operator claims
		operatorClaims.SystemAccount = systemAccountPubKey

		// Create system account claims
		sysAccountClaims := jwt.NewAccountClaims(systemAccountPubKey)
		sysAccountName := "SYS"
		if !data.SystemAccountName.IsNull() && !data.SystemAccountName.IsUnknown() {
			sysAccountName = data.SystemAccountName.ValueString()
		}
		sysAccountClaims.Name = sysAccountName
		sysAccountClaims.Issuer = operatorPubKey

		// Sign system account JWT with operator key
		systemAccountJWT, err = sysAccountClaims.Encode(operatorKP)
		if err != nil {
			resp.Diagnostics.AddError("Failed to encode system account JWT", err.Error())
			return
		}
	}

	// Sign the JWT
	operatorJWT, err := operatorClaims.Encode(operatorKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode operator JWT", err.Error())
		return
	}

	// Set computed values
	data.ID = types.StringValue(operatorPubKey)
	data.PublicKey = types.StringValue(operatorPubKey)
	data.Seed = types.StringValue(string(operatorSeed))
	data.JWT = types.StringValue(operatorJWT)

	if data.GenerateSigningKey.ValueBool() {
		data.SigningKeySeed = types.StringValue(string(signingKeySeed))
		data.SigningKey = types.StringValue(signingKeyPub)
	} else {
		data.SigningKeySeed = types.StringNull()
		data.SigningKey = types.StringNull()
	}

	// Set system account values
	if data.CreateSystemAccount.ValueBool() {
		data.SystemAccount = types.StringValue(systemAccountPubKey)
		data.SystemAccountJWT = types.StringValue(systemAccountJWT)
		data.SystemAccountSeed = types.StringValue(string(systemAccountSeed))
	} else {
		data.SystemAccount = types.StringNull()
		data.SystemAccountJWT = types.StringNull()
		data.SystemAccountSeed = types.StringNull()
	}

	tflog.Trace(ctx, "created operator resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OperatorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OperatorResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For state-only storage, nothing to read externally
	// JWT remains valid in state
}

func (r *OperatorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OperatorResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state to preserve keys
	var state OperatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Recreate operator key from seed
	operatorKP, err := nkeys.FromSeed([]byte(state.Seed.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore operator keypair", err.Error())
		return
	}

	operatorPubKey, err := operatorKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get operator public key", err.Error())
		return
	}

	// Create new operator claims with updated values
	operatorClaims := jwt.NewOperatorClaims(operatorPubKey)
	operatorClaims.Name = data.Name.ValueString()

	// Handle expiry
	if !data.Expiry.IsNull() && !data.Expiry.IsUnknown() {
		duration, diags := data.Expiry.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			operatorClaims.Expires = time.Now().Add(duration).Unix()
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
			operatorClaims.NotBefore = time.Now().Add(duration).Unix()
		}
	}

	// Preserve signing key if it exists
	if !state.SigningKey.IsNull() {
		operatorClaims.SigningKeys.Add(state.SigningKey.ValueString())
	}

	// Handle system account
	if data.CreateSystemAccount.ValueBool() {
		// If we have an existing system account, preserve it
		if !state.SystemAccount.IsNull() {
			operatorClaims.SystemAccount = state.SystemAccount.ValueString()
		} else {
			// Create new system account if it doesn't exist
			sysAccountKP, err := nkeys.CreateAccount()
			if err != nil {
				resp.Diagnostics.AddError("Failed to create system account keypair", err.Error())
				return
			}

			systemAccountPubKey, err := sysAccountKP.PublicKey()
			if err != nil {
				resp.Diagnostics.AddError("Failed to get system account public key", err.Error())
				return
			}

			systemAccountSeed, err := sysAccountKP.Seed()
			if err != nil {
				resp.Diagnostics.AddError("Failed to get system account seed", err.Error())
				return
			}

			operatorClaims.SystemAccount = systemAccountPubKey

			// Create system account claims
			sysAccountClaims := jwt.NewAccountClaims(systemAccountPubKey)
			sysAccountName := "SYS"
			if !data.SystemAccountName.IsNull() && !data.SystemAccountName.IsUnknown() {
				sysAccountName = data.SystemAccountName.ValueString()
			}
			sysAccountClaims.Name = sysAccountName
			sysAccountClaims.Issuer = operatorPubKey

			// Sign system account JWT
			systemAccountJWT, err := sysAccountClaims.Encode(operatorKP)
			if err != nil {
				resp.Diagnostics.AddError("Failed to encode system account JWT", err.Error())
				return
			}

			data.SystemAccount = types.StringValue(systemAccountPubKey)
			data.SystemAccountJWT = types.StringValue(systemAccountJWT)
			data.SystemAccountSeed = types.StringValue(string(systemAccountSeed))
		}
		// Regenerate system account JWT if exists
		if !state.SystemAccountSeed.IsNull() {
			// Validate we can restore the system account key
			_, err := nkeys.FromSeed([]byte(state.SystemAccountSeed.ValueString()))
			if err != nil {
				resp.Diagnostics.AddError("Failed to restore system account keypair", err.Error())
				return
			}

			// Recreate system account claims
			sysAccountClaims := jwt.NewAccountClaims(state.SystemAccount.ValueString())
			sysAccountName := "SYS"
			if !data.SystemAccountName.IsNull() && !data.SystemAccountName.IsUnknown() {
				sysAccountName = data.SystemAccountName.ValueString()
			}
			sysAccountClaims.Name = sysAccountName
			sysAccountClaims.Issuer = operatorPubKey

			// Re-sign system account JWT
			systemAccountJWT, err := sysAccountClaims.Encode(operatorKP)
			if err != nil {
				resp.Diagnostics.AddError("Failed to encode system account JWT", err.Error())
				return
			}

			data.SystemAccountJWT = types.StringValue(systemAccountJWT)
		}
	} else {
		// Clear system account if not requested
		data.SystemAccount = types.StringNull()
		data.SystemAccountJWT = types.StringNull()
		data.SystemAccountSeed = types.StringNull()
	}

	// Sign the JWT
	operatorJWT, err := operatorClaims.Encode(operatorKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode operator JWT", err.Error())
		return
	}

	// Update JWT while preserving keys
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Seed = state.Seed
	data.JWT = types.StringValue(operatorJWT)
	data.SigningKeySeed = state.SigningKeySeed
	data.SigningKey = state.SigningKey

	// Preserve system account if not set above
	if data.SystemAccount.IsNull() {
		data.SystemAccount = state.SystemAccount
		data.SystemAccountJWT = state.SystemAccountJWT
		data.SystemAccountSeed = state.SystemAccountSeed
	}

	tflog.Trace(ctx, "updated operator resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OperatorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OperatorResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Nothing to clean up - all data is in state
	tflog.Trace(ctx, "deleted operator resource")
}

func (r *OperatorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import formats:
	// - seed (just the operator seed)
	// - name/seed
	// - name/seed/signing_key_seed
	// Name can contain / encoded as // or %2F

	parts := strings.Split(req.ID, "/")

	var name string
	var operatorSeed string
	var signingKeySeed string

	// Parse from the end - seeds have predictable format
	// Last part should be operator seed (starts with SO)
	// Optional second-to-last could be signing key seed (also starts with SO)

	if len(parts) == 0 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be: seed, name/seed, or name/seed/signing_key_seed",
		)
		return
	}

	// Check if last part is a valid operator seed
	lastPart := parts[len(parts)-1]
	if !strings.HasPrefix(lastPart, "SO") {
		resp.Diagnostics.AddError(
			"Invalid operator seed",
			fmt.Sprintf("Expected operator seed starting with 'SO', got: %s", lastPart),
		)
		return
	}
	operatorSeed = lastPart

	// Check if we have a signing key seed
	if len(parts) >= 2 {
		secondLast := parts[len(parts)-2]
		if strings.HasPrefix(secondLast, "SO") {
			// It's a signing key seed
			signingKeySeed = secondLast
			// Name is everything before these two seeds
			if len(parts) > 2 {
				nameParts := parts[:len(parts)-2]
				name = strings.Join(nameParts, "/")
			}
		} else {
			// It's part of the name
			nameParts := parts[:len(parts)-1]
			name = strings.Join(nameParts, "/")
		}
	}

	// Decode name (handle // and %2F encodings)
	if name != "" {
		name = strings.ReplaceAll(name, "//", "\x00") // Temporary placeholder
		name = strings.ReplaceAll(name, "%2F", "/")
		name = strings.ReplaceAll(name, "\x00", "/") // Replace placeholder with /
	} else {
		name = "imported-operator"
	}

	// Validate the operator seed
	kp, err := nkeys.FromSeed([]byte(operatorSeed))
	if err != nil {
		resp.Diagnostics.AddError("Invalid operator seed", fmt.Sprintf("Failed to parse seed: %v", err))
		return
	}

	publicKey, err := kp.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Invalid keypair", fmt.Sprintf("Failed to get public key: %v", err))
		return
	}

	// Validate it's an operator key
	if !strings.HasPrefix(publicKey, "O") {
		resp.Diagnostics.AddError(
			"Invalid key type",
			fmt.Sprintf("Seed does not generate an operator public key (expected O*, got %s)", publicKey),
		)
		return
	}

	// Set state attributes
	resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("public_key"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("seed"), types.StringValue(operatorSeed))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))
	resp.State.SetAttribute(ctx, path.Root("generate_signing_key"), types.BoolValue(signingKeySeed != ""))
	resp.State.SetAttribute(ctx, path.Root("create_system_account"), types.BoolValue(false))
	resp.State.SetAttribute(ctx, path.Root("system_account_name"), types.StringValue("SYS"))
	resp.State.SetAttribute(ctx, path.Root("expiry"), timetypes.NewGoDurationValue(0))
	resp.State.SetAttribute(ctx, path.Root("start"), timetypes.NewGoDurationValue(0))

	// Generate a fresh JWT
	claims := jwt.NewOperatorClaims(publicKey)
	claims.Name = name

	// Handle signing key if provided
	if signingKeySeed != "" {
		signingKP, err := nkeys.FromSeed([]byte(signingKeySeed))
		if err != nil {
			resp.Diagnostics.AddError("Invalid signing key seed", fmt.Sprintf("Failed to parse signing seed: %v", err))
			return
		}

		signingPubKey, err := signingKP.PublicKey()
		if err != nil {
			resp.Diagnostics.AddError("Invalid signing keypair", fmt.Sprintf("Failed to get signing public key: %v", err))
			return
		}

		if !strings.HasPrefix(signingPubKey, "O") {
			resp.Diagnostics.AddError(
				"Invalid signing key type",
				fmt.Sprintf("Signing seed does not generate an operator public key (expected O*, got %s)", signingPubKey),
			)
			return
		}

		resp.State.SetAttribute(ctx, path.Root("signing_key_seed"), types.StringValue(signingKeySeed))
		resp.State.SetAttribute(ctx, path.Root("signing_key"), types.StringValue(signingPubKey))
		claims.SigningKeys.Add(signingPubKey)
	} else {
		resp.State.SetAttribute(ctx, path.Root("signing_key_seed"), types.StringNull())
		resp.State.SetAttribute(ctx, path.Root("signing_key"), types.StringNull())
	}

	// Encode JWT
	operatorJWT, err := claims.Encode(kp)
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate JWT", fmt.Sprintf("Failed to encode operator JWT: %v", err))
		return
	}
	resp.State.SetAttribute(ctx, path.Root("jwt"), types.StringValue(operatorJWT))

	// Set system account fields to null (no system account imported)
	resp.State.SetAttribute(ctx, path.Root("system_account"), types.StringNull())
	resp.State.SetAttribute(ctx, path.Root("system_account_jwt"), types.StringNull())
	resp.State.SetAttribute(ctx, path.Root("system_account_seed"), types.StringNull())
}
