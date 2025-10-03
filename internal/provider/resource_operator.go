package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &OperatorResource{}

func NewOperatorResource() resource.Resource {
	return &OperatorResource{}
}

type OperatorResource struct{}

type OperatorResourceModel struct {
	ID            types.String         `tfsdk:"id"`
	Name          types.String         `tfsdk:"name"`
	Subject       types.String         `tfsdk:"subject"`
	IssuerSeed    types.String         `tfsdk:"issuer_seed"`
	SigningKeys   types.List           `tfsdk:"signing_keys"`
	SystemAccount types.String         `tfsdk:"system_account"`
	ExpiresIn timetypes.GoDuration `tfsdk:"expires_in"`
	ExpiresAt timetypes.RFC3339    `tfsdk:"expires_at"`
	StartsIn  timetypes.GoDuration `tfsdk:"starts_in"`
	StartsAt  timetypes.RFC3339    `tfsdk:"starts_at"`
	JWT       types.String         `tfsdk:"jwt"`
	PublicKey types.String         `tfsdk:"public_key"`
}

func (r *OperatorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_operator"
}

func (r *OperatorResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a NATS JWT Operator. Use with nsc_nkey for key generation.",

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
			"subject": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Operator public key (subject of the JWT)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"issuer_seed": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Operator seed for signing the JWT (issuer). For operators, this is the same as subject's seed (self-issued).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"signing_keys": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Optional signing key public keys (for signing account JWTs)",
			},
			"system_account": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "System account public key reference",
			},
			"expires_in": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				MarkdownDescription: "Relative expiry duration (e.g., '8760h' for 1 year). Mutually exclusive with expires_at.",
			},
			"expires_at": schema.StringAttribute{
				CustomType:          timetypes.RFC3339Type{},
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Absolute expiry timestamp (RFC3339). Can be specified directly or computed from expires_in. Mutually exclusive with expires_in.",
			},
			"starts_in": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				MarkdownDescription: "Relative start delay (e.g., '72h' for 3 days). Mutually exclusive with starts_at.",
			},
			"starts_at": schema.StringAttribute{
				CustomType:          timetypes.RFC3339Type{},
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Absolute start timestamp (RFC3339). Can be specified directly or computed from starts_in. Mutually exclusive with starts_in.",
			},
			"jwt": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Generated JWT token",
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operator public key (same as subject)",
			},
		},
	}
}

func (r *OperatorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data OperatorResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate expiry attributes are mutually exclusive
	if !data.ExpiresIn.IsNull() && !data.ExpiresIn.IsUnknown() && !data.ExpiresAt.IsNull() && !data.ExpiresAt.IsUnknown() {
		resp.Diagnostics.AddError(
			"Conflicting Expiry Configuration",
			"Only one of 'expires_in' or 'expires_at' can be specified.",
		)
	}

	// Validate start attributes are mutually exclusive
	if !data.StartsIn.IsNull() && !data.StartsIn.IsUnknown() && !data.StartsAt.IsNull() && !data.StartsAt.IsUnknown() {
		resp.Diagnostics.AddError(
			"Conflicting Start Configuration",
			"Only one of 'starts_in' or 'starts_at' can be specified.",
		)
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

	// Get operator public key (subject)
	operatorPubKey := data.Subject.ValueString()
	if !strings.HasPrefix(operatorPubKey, "O") {
		resp.Diagnostics.AddError(
			"Invalid operator public key",
			fmt.Sprintf("Operator public key must start with 'O', got: %s", operatorPubKey[:1]),
		)
		return
	}

	// Get operator seed (issuer) for self-signing
	operatorSeedStr := data.IssuerSeed.ValueString()
	if !strings.HasPrefix(operatorSeedStr, "SO") {
		resp.Diagnostics.AddError(
			"Invalid operator seed",
			fmt.Sprintf("Operator seed must start with 'SO', got: %s", operatorSeedStr[:2]),
		)
		return
	}

	operatorKP, err := nkeys.FromSeed([]byte(operatorSeedStr))
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse operator seed", err.Error())
		return
	}

	// Verify the seed produces the expected public key
	verifyPubKey, err := operatorKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get public key from seed", err.Error())
		return
	}
	if verifyPubKey != operatorPubKey {
		resp.Diagnostics.AddError(
			"Key mismatch",
			fmt.Sprintf("Issuer seed produces public key %s, but subject is %s", verifyPubKey, operatorPubKey),
		)
		return
	}

	// Create operator claims
	operatorClaims := jwt.NewOperatorClaims(operatorPubKey)
	operatorClaims.Name = data.Name.ValueString()

	// Handle expiry (support old, new, and absolute variants)
	var expiresAtTime time.Time
	if !data.ExpiresIn.IsNull() && !data.ExpiresIn.IsUnknown() {
		// New relative duration - compute and store absolute
		duration, diags := data.ExpiresIn.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			expiresAtTime = time.Now().Add(duration)
			data.ExpiresAt = timetypes.NewRFC3339TimeValue(expiresAtTime)
			operatorClaims.Expires = expiresAtTime.Unix()
		} else {
			data.ExpiresAt = timetypes.NewRFC3339Null()
		}
	} else if !data.ExpiresAt.IsNull() && !data.ExpiresAt.IsUnknown() {
		// Absolute timestamp provided
		expiresAtTime, diags := data.ExpiresAt.ValueRFC3339Time()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		operatorClaims.Expires = expiresAtTime.Unix()
	} else {
		// No expiry specified - set to null
		data.ExpiresAt = timetypes.NewRFC3339Null()
	}

	// Handle start time (support old, new, and absolute variants)
	var startsAtTime time.Time
	if !data.StartsIn.IsNull() && !data.StartsIn.IsUnknown() {
		// New relative duration - compute and store absolute
		duration, diags := data.StartsIn.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			startsAtTime = time.Now().Add(duration)
			data.StartsAt = timetypes.NewRFC3339TimeValue(startsAtTime)
			operatorClaims.NotBefore = startsAtTime.Unix()
		} else {
			data.StartsAt = timetypes.NewRFC3339Null()
		}
	} else if !data.StartsAt.IsNull() && !data.StartsAt.IsUnknown() {
		// Absolute timestamp provided
		startsAtTime, diags := data.StartsAt.ValueRFC3339Time()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		operatorClaims.NotBefore = startsAtTime.Unix()
	} else {
		// No start time specified - set to null
		data.StartsAt = timetypes.NewRFC3339Null()
	}

	// Add signing keys if provided
	if !data.SigningKeys.IsNull() && !data.SigningKeys.IsUnknown() {
		var signingKeys []string
		resp.Diagnostics.Append(data.SigningKeys.ElementsAs(ctx, &signingKeys, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, key := range signingKeys {
			if !strings.HasPrefix(key, "O") {
				resp.Diagnostics.AddError(
					"Invalid signing key",
					fmt.Sprintf("Signing keys must be operator public keys (start with 'O'), got: %s", key),
				)
				return
			}
			operatorClaims.SigningKeys.Add(key)
		}
	}

	// Set system account if provided
	if !data.SystemAccount.IsNull() && !data.SystemAccount.IsUnknown() {
		systemAccountPubKey := data.SystemAccount.ValueString()
		if !strings.HasPrefix(systemAccountPubKey, "A") {
			resp.Diagnostics.AddError(
				"Invalid system account",
				fmt.Sprintf("System account must be an account public key (start with 'A'), got: %s", systemAccountPubKey),
			)
			return
		}
		operatorClaims.SystemAccount = systemAccountPubKey
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
	data.JWT = types.StringValue(operatorJWT)

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

	// Get current state to preserve immutable fields
	var state OperatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get operator public key and seed from state (immutable)
	operatorPubKey := state.Subject.ValueString()
	operatorSeedStr := state.IssuerSeed.ValueString()

	operatorKP, err := nkeys.FromSeed([]byte(operatorSeedStr))
	if err != nil {
		resp.Diagnostics.AddError("Failed to restore operator keypair", err.Error())
		return
	}

	// Create new operator claims with updated values
	operatorClaims := jwt.NewOperatorClaims(operatorPubKey)
	operatorClaims.Name = data.Name.ValueString()

	// Handle expiry (support old, new, and absolute variants)
	var expiresAtTime time.Time
	if !data.ExpiresIn.IsNull() && !data.ExpiresIn.IsUnknown() {
		// New relative duration - compute and store absolute
		duration, diags := data.ExpiresIn.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			expiresAtTime = time.Now().Add(duration)
			data.ExpiresAt = timetypes.NewRFC3339TimeValue(expiresAtTime)
			operatorClaims.Expires = expiresAtTime.Unix()
		} else {
			data.ExpiresAt = timetypes.NewRFC3339Null()
		}
	} else if !data.ExpiresAt.IsNull() && !data.ExpiresAt.IsUnknown() {
		// Absolute timestamp provided
		expiresAtTime, diags := data.ExpiresAt.ValueRFC3339Time()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		operatorClaims.Expires = expiresAtTime.Unix()
	} else {
		// No expiry specified - set to null
		data.ExpiresAt = timetypes.NewRFC3339Null()
	}

	// Handle start time (support old, new, and absolute variants)
	var startsAtTime time.Time
	if !data.StartsIn.IsNull() && !data.StartsIn.IsUnknown() {
		// New relative duration - compute and store absolute
		duration, diags := data.StartsIn.ValueGoDuration()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if duration != 0 {
			startsAtTime = time.Now().Add(duration)
			data.StartsAt = timetypes.NewRFC3339TimeValue(startsAtTime)
			operatorClaims.NotBefore = startsAtTime.Unix()
		} else {
			data.StartsAt = timetypes.NewRFC3339Null()
		}
	} else if !data.StartsAt.IsNull() && !data.StartsAt.IsUnknown() {
		// Absolute timestamp provided
		startsAtTime, diags := data.StartsAt.ValueRFC3339Time()
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		operatorClaims.NotBefore = startsAtTime.Unix()
	} else {
		// No start time specified - set to null
		data.StartsAt = timetypes.NewRFC3339Null()
	}

	// Add signing keys if provided
	if !data.SigningKeys.IsNull() && !data.SigningKeys.IsUnknown() {
		var signingKeys []string
		resp.Diagnostics.Append(data.SigningKeys.ElementsAs(ctx, &signingKeys, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, key := range signingKeys {
			if !strings.HasPrefix(key, "O") {
				resp.Diagnostics.AddError(
					"Invalid signing key",
					fmt.Sprintf("Signing keys must be operator public keys (start with 'O'), got: %s", key),
				)
				return
			}
			operatorClaims.SigningKeys.Add(key)
		}
	}

	// Set system account if provided
	if !data.SystemAccount.IsNull() && !data.SystemAccount.IsUnknown() {
		systemAccountPubKey := data.SystemAccount.ValueString()
		if !strings.HasPrefix(systemAccountPubKey, "A") {
			resp.Diagnostics.AddError(
				"Invalid system account",
				fmt.Sprintf("System account must be an account public key (start with 'A'), got: %s", systemAccountPubKey),
			)
			return
		}
		operatorClaims.SystemAccount = systemAccountPubKey
	}

	// Sign the JWT
	operatorJWT, err := operatorClaims.Encode(operatorKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode operator JWT", err.Error())
		return
	}

	// Update JWT while preserving immutable fields
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Subject = state.Subject
	data.IssuerSeed = state.IssuerSeed
	data.JWT = types.StringValue(operatorJWT)

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
