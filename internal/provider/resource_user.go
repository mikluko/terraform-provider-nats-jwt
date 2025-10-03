package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

var _ resource.Resource = &UserResource{}

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

	ExpiresIn    timetypes.GoDuration `tfsdk:"expires_in"`
	ExpiresAt    timetypes.RFC3339    `tfsdk:"expires_at"`
	StartsIn     timetypes.GoDuration `tfsdk:"starts_in"`
	StartsAt     timetypes.RFC3339    `tfsdk:"starts_at"`
	JWT          types.String         `tfsdk:"jwt"`
	JWTSensitive types.String         `tfsdk:"jwt_sensitive"`
	PublicKey    types.String         `tfsdk:"public_key"`
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
				MarkdownDescription: "Publish permissions. If not specified, inherits from account default permissions.",
			},
			"allow_sub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Subscribe permissions. If not specified, inherits from account default permissions.",
			},
			"deny_pub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Deny publish permissions. If not specified, inherits from account default permissions.",
			},
			"deny_sub": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Deny subscribe permissions. If not specified, inherits from account default permissions.",
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
			"expires_in": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				MarkdownDescription: "Relative expiry duration (e.g., '720h' for 30 days, '0s' for no expiry). Mutually exclusive with `expires_at`. JWT regenerates with new expiry on any resource change (rolling expiry).",
			},
			"expires_at": schema.StringAttribute{
				CustomType:          timetypes.RFC3339Type{},
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Absolute expiry timestamp in RFC3339 format (e.g., '2026-01-01T00:00:00Z'). Can be specified directly or computed from `expires_in`. Mutually exclusive with `expires_in`. Use this for fixed deadlines that won't change.",
			},
			"starts_in": schema.StringAttribute{
				CustomType:          timetypes.GoDurationType{},
				Optional:            true,
				MarkdownDescription: "Relative start duration (e.g., '24h' for 1 day from now, '0s' for immediately). Mutually exclusive with `starts_at`. JWT regenerates with new start time on any resource change.",
			},
			"starts_at": schema.StringAttribute{
				CustomType:          timetypes.RFC3339Type{},
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Absolute start timestamp in RFC3339 format (e.g., '2025-01-01T00:00:00Z'). Can be specified directly or computed from `starts_in`. Mutually exclusive with `starts_in`. Use this for fixed start times that won't change.",
			},
			"jwt": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Generated JWT token. Only populated when bearer = false. For bearer tokens, use jwt_sensitive instead.",
			},
			"jwt_sensitive": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Generated JWT token (always populated, marked as sensitive). Use this when bearer = true.",
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

func (r *UserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data UserResourceModel

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
			userClaims.Expires = expiresAtTime.Unix()
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
		userClaims.Expires = expiresAtTime.Unix()
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
			userClaims.NotBefore = startsAtTime.Unix()
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
		userClaims.NotBefore = startsAtTime.Unix()
	} else {
		// No start time specified - set to null
		data.StartsAt = timetypes.NewRFC3339Null()
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

	// Always populate jwt_sensitive
	data.JWTSensitive = types.StringValue(userJWT)

	// Only populate jwt when bearer = false (non-bearer tokens are not secrets)
	if !data.Bearer.ValueBool() {
		data.JWT = types.StringValue(userJWT)
	} else {
		data.JWT = types.StringNull()
	}

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
			userClaims.Expires = expiresAtTime.Unix()
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
		userClaims.Expires = expiresAtTime.Unix()
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
			userClaims.NotBefore = startsAtTime.Unix()
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
		userClaims.NotBefore = startsAtTime.Unix()
	} else {
		// No start time specified - set to null
		data.StartsAt = timetypes.NewRFC3339Null()
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

	// Always populate jwt_sensitive
	data.JWTSensitive = types.StringValue(userJWT)

	// Only populate jwt when bearer = false (non-bearer tokens are not secrets)
	if !data.Bearer.ValueBool() {
		data.JWT = types.StringValue(userJWT)
	} else {
		data.JWT = types.StringNull()
	}

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
