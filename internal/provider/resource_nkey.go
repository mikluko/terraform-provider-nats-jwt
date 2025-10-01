package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &NKeyResource{}
var _ resource.ResourceWithImportState = &NKeyResource{}

func NewNKeyResource() resource.Resource {
	return &NKeyResource{}
}

type NKeyResource struct{}

type NKeyResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Type      types.String `tfsdk:"type"`
	PublicKey types.String `tfsdk:"public_key"`
	Seed      types.String `tfsdk:"seed"`
}

func (r *NKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nkey"
}

func (r *NKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Generates a NATS NKey keypair. Use with nsc_operator, nsc_account, or nsc_user resources to create JWTs.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "NKey identifier (public key)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "NKey type: operator, account, or user",
				Validators: []validator.String{
					stringvalidator.OneOf("operator", "account", "user"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "NKey public key",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"seed": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "NKey seed (private key)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *NKeyResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	// No provider configuration needed
}

func (r *NKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create key pair based on type
	keyType := data.Type.ValueString()
	var kp nkeys.KeyPair
	var err error

	switch keyType {
	case "operator":
		kp, err = nkeys.CreateOperator()
	case "account":
		kp, err = nkeys.CreateAccount()
	case "user":
		kp, err = nkeys.CreateUser()
	default:
		resp.Diagnostics.AddError(
			"Invalid NKey type",
			fmt.Sprintf("Type must be one of: operator, account, user. Got: %s", keyType),
		)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError("Failed to create NKey", err.Error())
		return
	}

	publicKey, err := kp.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get public key", err.Error())
		return
	}

	seed, err := kp.Seed()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get seed", err.Error())
		return
	}

	// Validate the key type matches
	var expectedPrefix string
	switch keyType {
	case "operator":
		expectedPrefix = "O"
	case "account":
		expectedPrefix = "A"
	case "user":
		expectedPrefix = "U"
	}

	if !strings.HasPrefix(publicKey, expectedPrefix) {
		resp.Diagnostics.AddError(
			"Key type mismatch",
			fmt.Sprintf("Generated key does not match type %s (expected prefix %s, got %s)", keyType, expectedPrefix, publicKey[:1]),
		)
		return
	}

	// Set computed values
	data.ID = types.StringValue(publicKey)
	data.PublicKey = types.StringValue(publicKey)
	data.Seed = types.StringValue(string(seed))

	tflog.Trace(ctx, "created nkey resource", map[string]any{"type": keyType})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For state-only storage, nothing to read externally
	// Keys remain valid in state
}

func (r *NKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// NKeys are immutable - type has RequiresReplace modifier
	// This should never be called, but implement for completeness
	var data NKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError(
		"NKey Update Not Supported",
		"NKeys are immutable. Changing the type requires replacing the resource.",
	)
}

func (r *NKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Nothing to clean up - all data is in state
	tflog.Trace(ctx, "deleted nkey resource")
}

func (r *NKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: just the seed
	seedStr := req.ID

	// Parse the seed to determine type and validate
	kp, err := nkeys.FromSeed([]byte(seedStr))
	if err != nil {
		resp.Diagnostics.AddError("Invalid seed", fmt.Sprintf("Failed to parse seed: %v", err))
		return
	}

	publicKey, err := kp.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Invalid keypair", fmt.Sprintf("Failed to get public key: %v", err))
		return
	}

	// Determine type from public key prefix
	var keyType string
	var seedPrefix string
	switch {
	case strings.HasPrefix(publicKey, "O"):
		keyType = "operator"
		seedPrefix = "SO"
	case strings.HasPrefix(publicKey, "A"):
		keyType = "account"
		seedPrefix = "SA"
	case strings.HasPrefix(publicKey, "U"):
		keyType = "user"
		seedPrefix = "SU"
	default:
		resp.Diagnostics.AddError(
			"Invalid key type",
			fmt.Sprintf("Unknown key type from public key: %s", publicKey[:1]),
		)
		return
	}

	// Validate seed prefix matches
	if !strings.HasPrefix(seedStr, seedPrefix) {
		resp.Diagnostics.AddError(
			"Seed type mismatch",
			fmt.Sprintf("Seed prefix %s does not match expected %s for %s key", seedStr[:2], seedPrefix, keyType),
		)
		return
	}

	// Set state attributes
	resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("type"), types.StringValue(keyType))
	resp.State.SetAttribute(ctx, path.Root("public_key"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("seed"), types.StringValue(seedStr))
}
