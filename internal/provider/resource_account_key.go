package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &AccountKeyResource{}
var _ resource.ResourceWithImportState = &AccountKeyResource{}

func NewAccountKeyResource() resource.Resource {
	return &AccountKeyResource{}
}

type AccountKeyResource struct{}

type AccountKeyResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Seed      types.String `tfsdk:"seed"`
	PublicKey types.String `tfsdk:"public_key"`
}

func (r *AccountKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_key"
}

func (r *AccountKeyResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Generates a NATS account keypair without creating a JWT. Use with nsc_account_jwt to resolve circular dependencies in cross-account imports.",

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
			"seed": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Account seed (private key). If provided, imports an existing key; otherwise generates a new one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account public key",
			},
		},
	}
}

func (r *AccountKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// No provider configuration needed
}

func (r *AccountKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AccountKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var accountKP nkeys.KeyPair
	var err error

	// Check if seed is provided (import existing key)
	if !data.Seed.IsNull() && !data.Seed.IsUnknown() {
		seedStr := data.Seed.ValueString()

		// Validate seed format
		if !strings.HasPrefix(seedStr, "SA") {
			resp.Diagnostics.AddError(
				"Invalid account seed",
				fmt.Sprintf("Account seed must start with 'SA', got: %s", seedStr[:2]),
			)
			return
		}

		accountKP, err = nkeys.FromSeed([]byte(seedStr))
		if err != nil {
			resp.Diagnostics.AddError("Failed to parse provided seed", err.Error())
			return
		}
	} else {
		// Generate new account key pair
		accountKP, err = nkeys.CreateAccount()
		if err != nil {
			resp.Diagnostics.AddError("Failed to create account keypair", err.Error())
			return
		}
	}

	accountPubKey, err := accountKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account public key", err.Error())
		return
	}

	// Validate it's an account key
	if !strings.HasPrefix(accountPubKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid key type",
			fmt.Sprintf("Seed does not generate an account public key (expected A*, got %s)", accountPubKey),
		)
		return
	}

	accountSeed, err := accountKP.Seed()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get account seed", err.Error())
		return
	}

	// Set computed values
	data.ID = types.StringValue(accountPubKey)
	data.PublicKey = types.StringValue(accountPubKey)
	data.Seed = types.StringValue(string(accountSeed))

	tflog.Trace(ctx, "created account key resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AccountKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For state-only storage, nothing to read externally
	// Keys remain valid in state
}

func (r *AccountKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AccountKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current state to preserve keys
	var state AccountKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only name can be updated - keys are immutable
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Seed = state.Seed

	tflog.Trace(ctx, "updated account key resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccountKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AccountKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Nothing to clean up - all data is in state
	tflog.Trace(ctx, "deleted account key resource")
}

func (r *AccountKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import formats:
	// - seed (just the account seed)
	// - name/seed
	// Name can contain / encoded as // or %2F

	parts := strings.Split(req.ID, "/")

	var name string
	var accountSeed string

	// Parse from the end - seeds have predictable format
	// Last part should be account seed (starts with SA)

	if len(parts) == 0 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be: seed or name/seed",
		)
		return
	}

	// Check if last part is a valid account seed
	lastPart := parts[len(parts)-1]
	if !strings.HasPrefix(lastPart, "SA") {
		resp.Diagnostics.AddError(
			"Invalid account seed",
			fmt.Sprintf("Expected account seed starting with 'SA', got: %s", lastPart),
		)
		return
	}
	accountSeed = lastPart

	// Name is everything before the seed
	if len(parts) > 1 {
		nameParts := parts[:len(parts)-1]
		name = strings.Join(nameParts, "/")
	}

	// Decode name (handle // and %2F encodings)
	if name != "" {
		name = strings.ReplaceAll(name, "//", "\x00") // Temporary placeholder
		name = strings.ReplaceAll(name, "%2F", "/")
		name = strings.ReplaceAll(name, "\x00", "/") // Replace placeholder with /
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

	// Set state attributes
	resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("public_key"), types.StringValue(publicKey))
	resp.State.SetAttribute(ctx, path.Root("seed"), types.StringValue(accountSeed))
	resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(name))
}
