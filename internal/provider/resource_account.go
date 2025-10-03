package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
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

func NewAccountResource() resource.Resource {
	return &AccountResource{}
}

type AccountResource struct{}

type ExportModel struct {
	Name                 types.String         `tfsdk:"name"`
	Subject              types.String         `tfsdk:"subject"`
	Type                 types.String         `tfsdk:"type"`
	TokenRequired        types.Bool           `tfsdk:"token_required"`
	ResponseType         types.String         `tfsdk:"response_type"`
	ResponseThreshold    timetypes.GoDuration `tfsdk:"response_threshold"`
	AccountTokenPosition types.Int64          `tfsdk:"account_token_position"`
	Advertise            types.Bool           `tfsdk:"advertise"`
	AllowTrace           types.Bool           `tfsdk:"allow_trace"`
	Description          types.String         `tfsdk:"description"`
	InfoURL              types.String         `tfsdk:"info_url"`
}

type ImportModel struct {
	Name         types.String `tfsdk:"name"`
	Subject      types.String `tfsdk:"subject"`
	Account      types.String `tfsdk:"account"`
	Token        types.String `tfsdk:"token"`
	LocalSubject types.String `tfsdk:"local_subject"`
	Type         types.String `tfsdk:"type"`
	Share        types.Bool   `tfsdk:"share"`
	AllowTrace   types.Bool   `tfsdk:"allow_trace"`
}

type AccountResourceModel struct {
	ID               types.String         `tfsdk:"id"`
	Name             types.String         `tfsdk:"name"`
	Subject          types.String         `tfsdk:"subject"`
	IssuerSeed       types.String         `tfsdk:"issuer_seed"`
	SigningKeys      types.List           `tfsdk:"signing_keys"`
	AllowPub         types.List           `tfsdk:"allow_pub"`
	AllowSub         types.List           `tfsdk:"allow_sub"`
	DenyPub          types.List           `tfsdk:"deny_pub"`
	DenySub          types.List           `tfsdk:"deny_sub"`
	AllowPubResponse types.Int64          `tfsdk:"allow_pub_response"`
	ResponseTTL      timetypes.GoDuration `tfsdk:"response_ttl"`
	Expiry           timetypes.GoDuration `tfsdk:"expiry"`
	Start            timetypes.GoDuration `tfsdk:"start"`

	// Account Limits
	MaxConnections       types.Int64 `tfsdk:"max_connections"`
	MaxLeafNodes         types.Int64 `tfsdk:"max_leaf_nodes"`
	MaxData              types.Int64 `tfsdk:"max_data"`
	MaxPayload           types.Int64 `tfsdk:"max_payload"`
	MaxSubscriptions     types.Int64 `tfsdk:"max_subscriptions"`
	MaxImports           types.Int64 `tfsdk:"max_imports"`
	MaxExports           types.Int64 `tfsdk:"max_exports"`
	AllowWildcardExports types.Bool  `tfsdk:"allow_wildcard_exports"`
	DisallowBearerToken  types.Bool  `tfsdk:"disallow_bearer_token"`

	// JetStream Limits
	MaxMemoryStorage     types.Int64 `tfsdk:"max_memory_storage"`
	MaxDiskStorage       types.Int64 `tfsdk:"max_disk_storage"`
	MaxStreams           types.Int64 `tfsdk:"max_streams"`
	MaxConsumers         types.Int64 `tfsdk:"max_consumers"`
	MaxAckPending        types.Int64 `tfsdk:"max_ack_pending"`
	MaxMemoryStreamBytes types.Int64 `tfsdk:"max_memory_stream_bytes"`
	MaxDiskStreamBytes   types.Int64 `tfsdk:"max_disk_stream_bytes"`
	MaxBytesRequired     types.Bool  `tfsdk:"max_bytes_required"`

	// Imports/Exports
	Exports types.List `tfsdk:"export"`
	Imports types.List `tfsdk:"import"`

	JWT       types.String `tfsdk:"jwt"`
	PublicKey types.String `tfsdk:"public_key"`
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
			"subject": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Account public key (subject of the JWT)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"issuer_seed": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Operator seed for signing the account JWT (issuer)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"signing_keys": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Optional signing key public keys (for signing user JWTs)",
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
				MarkdownDescription: "Generated JWT token",
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account public key",
			},

			// Account Limits
			"max_connections": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of active connections (-1 for unlimited)",
			},
			"max_leaf_nodes": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of active leaf node connections (-1 for unlimited)",
			},
			"max_data": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of bytes (-1 for unlimited)",
			},
			"max_payload": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum message payload in bytes (-1 for unlimited)",
			},
			"max_subscriptions": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of subscriptions (-1 for unlimited)",
			},
			"max_imports": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of imports (-1 for unlimited)",
			},
			"max_exports": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of exports (-1 for unlimited)",
			},
			"allow_wildcard_exports": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Allow wildcards in exports",
			},
			"disallow_bearer_token": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Disallow user JWTs to be bearer tokens",
			},

			// JetStream Limits
			"max_memory_storage": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum bytes stored in memory across all streams (0 for disabled)",
			},
			"max_disk_storage": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum bytes stored on disk across all streams (0 for disabled)",
			},
			"max_streams": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of streams (-1 for unlimited)",
			},
			"max_consumers": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of consumers (-1 for unlimited)",
			},
			"max_ack_pending": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum ack pending of a stream (-1 for unlimited)",
			},
			"max_memory_stream_bytes": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum bytes a memory backed stream can have (0 for unlimited)",
			},
			"max_disk_stream_bytes": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum bytes a disk backed stream can have (0 for unlimited)",
			},
			"max_bytes_required": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Require max bytes to be set for all streams",
			},
		},
		Blocks: map[string]schema.Block{
			"export": schema.ListNestedBlock{
				MarkdownDescription: "Exports this account provides to other accounts",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Export name",
						},
						"subject": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Subject pattern to export",
						},
						"type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Export type: 'stream' for pub/sub or 'service' for request/reply",
						},
						"token_required": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Whether importing accounts need an activation token",
						},
						"response_type": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Service response type: 'Singleton' (single response), 'Stream' (multiple responses), or 'Chunked' (chunked single response)",
						},
						"response_threshold": schema.StringAttribute{
							CustomType:          timetypes.GoDurationType{},
							Optional:            true,
							MarkdownDescription: "Maximum time to wait for service response (e.g., '5s')",
						},
						"account_token_position": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Position in the subject where the account token appears (for multi-tenant exports)",
						},
						"advertise": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Advertise this export publicly",
						},
						"allow_trace": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Allow tracing for this export",
						},
						"description": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Export description",
						},
						"info_url": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "URL with more information about this export",
						},
					},
				},
			},
			"import": schema.ListNestedBlock{
				MarkdownDescription: "Imports from other accounts",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Import name",
						},
						"subject": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Subject pattern from the exporting account's perspective",
						},
						"account": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Public key of the exporting account",
						},
						"token": schema.StringAttribute{
							Optional:            true,
							Sensitive:           true,
							MarkdownDescription: "Activation token if required by the export",
						},
						"local_subject": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Local subject mapping (can use $1, $2 for wildcard references)",
						},
						"type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Import type: 'stream' for pub/sub or 'service' for request/reply",
						},
						"share": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Share imported service across queue subscribers",
						},
						"allow_trace": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Allow tracing for this import",
						},
					},
				},
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

	// Get account public key (subject)
	accountPubKey := data.Subject.ValueString()
	if !strings.HasPrefix(accountPubKey, "A") {
		resp.Diagnostics.AddError(
			"Invalid account public key",
			fmt.Sprintf("Account public key must start with 'A', got: %s", accountPubKey[:1]),
		)
		return
	}

	// Get operator seed (issuer) for signing
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

	operatorPubKey, err := operatorKP.PublicKey()
	if err != nil {
		resp.Diagnostics.AddError("Failed to get operator public key", err.Error())
		return
	}

	// Validate it's actually an operator key
	if !strings.HasPrefix(operatorPubKey, "O") {
		resp.Diagnostics.AddError(
			"Invalid operator seed",
			fmt.Sprintf("Seed does not generate an operator public key (expected O*, got %s)", operatorPubKey),
		)
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

	// Set Account Limits
	if !data.MaxConnections.IsNull() {
		accountClaims.Limits.Conn = data.MaxConnections.ValueInt64()
	}
	if !data.MaxLeafNodes.IsNull() {
		accountClaims.Limits.LeafNodeConn = data.MaxLeafNodes.ValueInt64()
	}
	if !data.MaxData.IsNull() {
		accountClaims.Limits.Data = data.MaxData.ValueInt64()
	}
	if !data.MaxPayload.IsNull() {
		accountClaims.Limits.Payload = data.MaxPayload.ValueInt64()
	}
	if !data.MaxSubscriptions.IsNull() {
		accountClaims.Limits.Subs = data.MaxSubscriptions.ValueInt64()
	}
	if !data.MaxImports.IsNull() {
		accountClaims.Limits.Imports = data.MaxImports.ValueInt64()
	}
	if !data.MaxExports.IsNull() {
		accountClaims.Limits.Exports = data.MaxExports.ValueInt64()
	}
	if !data.AllowWildcardExports.IsNull() {
		accountClaims.Limits.WildcardExports = data.AllowWildcardExports.ValueBool()
	}
	if !data.DisallowBearerToken.IsNull() {
		accountClaims.Limits.DisallowBearer = data.DisallowBearerToken.ValueBool()
	}

	// Set JetStream Limits
	if !data.MaxMemoryStorage.IsNull() {
		accountClaims.Limits.MemoryStorage = data.MaxMemoryStorage.ValueInt64()
	}
	if !data.MaxDiskStorage.IsNull() {
		accountClaims.Limits.DiskStorage = data.MaxDiskStorage.ValueInt64()
	}
	if !data.MaxStreams.IsNull() {
		accountClaims.Limits.Streams = data.MaxStreams.ValueInt64()
	}
	if !data.MaxConsumers.IsNull() {
		accountClaims.Limits.Consumer = data.MaxConsumers.ValueInt64()
	}
	if !data.MaxAckPending.IsNull() {
		accountClaims.Limits.MaxAckPending = data.MaxAckPending.ValueInt64()
	}
	if !data.MaxMemoryStreamBytes.IsNull() {
		accountClaims.Limits.MemoryMaxStreamBytes = data.MaxMemoryStreamBytes.ValueInt64()
	}
	if !data.MaxDiskStreamBytes.IsNull() {
		accountClaims.Limits.DiskMaxStreamBytes = data.MaxDiskStreamBytes.ValueInt64()
	}
	if !data.MaxBytesRequired.IsNull() {
		accountClaims.Limits.MaxBytesRequired = data.MaxBytesRequired.ValueBool()
	}

	// Handle exports
	if !data.Exports.IsNull() && len(data.Exports.Elements()) > 0 {
		var exports []ExportModel
		resp.Diagnostics.Append(data.Exports.ElementsAs(ctx, &exports, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, export := range exports {
			jwtExport := &jwt.Export{
				Subject: jwt.Subject(export.Subject.ValueString()),
			}

			// Set export type
			switch export.Type.ValueString() {
			case "stream":
				jwtExport.Type = jwt.Stream
			case "service":
				jwtExport.Type = jwt.Service
			default:
				resp.Diagnostics.AddError(
					"Invalid export type",
					fmt.Sprintf("Export type must be 'stream' or 'service', got: %s", export.Type.ValueString()),
				)
				return
			}

			// Optional fields
			if !export.Name.IsNull() {
				jwtExport.Name = export.Name.ValueString()
			}
			if !export.TokenRequired.IsNull() {
				jwtExport.TokenReq = export.TokenRequired.ValueBool()
			}
			if !export.ResponseType.IsNull() {
				jwtExport.ResponseType = jwt.ResponseType(export.ResponseType.ValueString())
			}
			if !export.ResponseThreshold.IsNull() && !export.ResponseThreshold.IsUnknown() {
				duration, diags := export.ResponseThreshold.ValueGoDuration()
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
				jwtExport.ResponseThreshold = duration
			}
			if !export.AccountTokenPosition.IsNull() {
				jwtExport.AccountTokenPosition = uint(export.AccountTokenPosition.ValueInt64())
			}
			if !export.Advertise.IsNull() {
				jwtExport.Advertise = export.Advertise.ValueBool()
			}
			if !export.AllowTrace.IsNull() {
				jwtExport.AllowTrace = export.AllowTrace.ValueBool()
			}
			if !export.Description.IsNull() {
				jwtExport.Description = export.Description.ValueString()
			}
			if !export.InfoURL.IsNull() {
				jwtExport.InfoURL = export.InfoURL.ValueString()
			}

			accountClaims.Exports.Add(jwtExport)
		}
	}

	// Handle imports
	if !data.Imports.IsNull() && len(data.Imports.Elements()) > 0 {
		var imports []ImportModel
		resp.Diagnostics.Append(data.Imports.ElementsAs(ctx, &imports, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, imp := range imports {
			jwtImport := &jwt.Import{
				Subject: jwt.Subject(imp.Subject.ValueString()),
				Account: imp.Account.ValueString(),
			}

			// Set import type
			switch imp.Type.ValueString() {
			case "stream":
				jwtImport.Type = jwt.Stream
			case "service":
				jwtImport.Type = jwt.Service
			default:
				resp.Diagnostics.AddError(
					"Invalid import type",
					fmt.Sprintf("Import type must be 'stream' or 'service', got: %s", imp.Type.ValueString()),
				)
				return
			}

			// Optional fields
			if !imp.Name.IsNull() {
				jwtImport.Name = imp.Name.ValueString()
			}
			if !imp.Token.IsNull() {
				jwtImport.Token = imp.Token.ValueString()
			}
			if !imp.LocalSubject.IsNull() {
				jwtImport.LocalSubject = jwt.RenamingSubject(imp.LocalSubject.ValueString())
			}
			if !imp.Share.IsNull() {
				jwtImport.Share = imp.Share.ValueBool()
			}
			if !imp.AllowTrace.IsNull() {
				jwtImport.AllowTrace = imp.AllowTrace.ValueBool()
			}

			accountClaims.Imports.Add(jwtImport)
		}
	}

	// Add signing keys if provided
	if !data.SigningKeys.IsNull() && !data.SigningKeys.IsUnknown() {
		var signingKeys []string
		resp.Diagnostics.Append(data.SigningKeys.ElementsAs(ctx, &signingKeys, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, key := range signingKeys {
			if !strings.HasPrefix(key, "A") {
				resp.Diagnostics.AddError(
					"Invalid signing key",
					fmt.Sprintf("Signing keys must be account public keys (start with 'A'), got: %s", key),
				)
				return
			}
			accountClaims.SigningKeys.Add(key)
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
	data.JWT = types.StringValue(accountJWT)

	tflog.Trace(ctx, "created account resource")

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

	// Get current state to preserve immutable fields
	var state AccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get account public key and operator seed from state (immutable)
	accountPubKey := state.Subject.ValueString()
	operatorSeedStr := state.IssuerSeed.ValueString()

	operatorKP, err := nkeys.FromSeed([]byte(operatorSeedStr))
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
	accountClaims := jwt.NewAccountClaims(accountPubKey)
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

	// Set Account Limits
	if !data.MaxConnections.IsNull() {
		accountClaims.Limits.Conn = data.MaxConnections.ValueInt64()
	}
	if !data.MaxLeafNodes.IsNull() {
		accountClaims.Limits.LeafNodeConn = data.MaxLeafNodes.ValueInt64()
	}
	if !data.MaxData.IsNull() {
		accountClaims.Limits.Data = data.MaxData.ValueInt64()
	}
	if !data.MaxPayload.IsNull() {
		accountClaims.Limits.Payload = data.MaxPayload.ValueInt64()
	}
	if !data.MaxSubscriptions.IsNull() {
		accountClaims.Limits.Subs = data.MaxSubscriptions.ValueInt64()
	}
	if !data.MaxImports.IsNull() {
		accountClaims.Limits.Imports = data.MaxImports.ValueInt64()
	}
	if !data.MaxExports.IsNull() {
		accountClaims.Limits.Exports = data.MaxExports.ValueInt64()
	}
	if !data.AllowWildcardExports.IsNull() {
		accountClaims.Limits.WildcardExports = data.AllowWildcardExports.ValueBool()
	}
	if !data.DisallowBearerToken.IsNull() {
		accountClaims.Limits.DisallowBearer = data.DisallowBearerToken.ValueBool()
	}

	// Set JetStream Limits
	if !data.MaxMemoryStorage.IsNull() {
		accountClaims.Limits.MemoryStorage = data.MaxMemoryStorage.ValueInt64()
	}
	if !data.MaxDiskStorage.IsNull() {
		accountClaims.Limits.DiskStorage = data.MaxDiskStorage.ValueInt64()
	}
	if !data.MaxStreams.IsNull() {
		accountClaims.Limits.Streams = data.MaxStreams.ValueInt64()
	}
	if !data.MaxConsumers.IsNull() {
		accountClaims.Limits.Consumer = data.MaxConsumers.ValueInt64()
	}
	if !data.MaxAckPending.IsNull() {
		accountClaims.Limits.MaxAckPending = data.MaxAckPending.ValueInt64()
	}
	if !data.MaxMemoryStreamBytes.IsNull() {
		accountClaims.Limits.MemoryMaxStreamBytes = data.MaxMemoryStreamBytes.ValueInt64()
	}
	if !data.MaxDiskStreamBytes.IsNull() {
		accountClaims.Limits.DiskMaxStreamBytes = data.MaxDiskStreamBytes.ValueInt64()
	}
	if !data.MaxBytesRequired.IsNull() {
		accountClaims.Limits.MaxBytesRequired = data.MaxBytesRequired.ValueBool()
	}

	// Handle exports
	if !data.Exports.IsNull() && len(data.Exports.Elements()) > 0 {
		var exports []ExportModel
		resp.Diagnostics.Append(data.Exports.ElementsAs(ctx, &exports, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, export := range exports {
			jwtExport := &jwt.Export{
				Subject: jwt.Subject(export.Subject.ValueString()),
			}

			// Set export type
			switch export.Type.ValueString() {
			case "stream":
				jwtExport.Type = jwt.Stream
			case "service":
				jwtExport.Type = jwt.Service
			default:
				resp.Diagnostics.AddError(
					"Invalid export type",
					fmt.Sprintf("Export type must be 'stream' or 'service', got: %s", export.Type.ValueString()),
				)
				return
			}

			// Optional fields
			if !export.Name.IsNull() {
				jwtExport.Name = export.Name.ValueString()
			}
			if !export.TokenRequired.IsNull() {
				jwtExport.TokenReq = export.TokenRequired.ValueBool()
			}
			if !export.ResponseType.IsNull() {
				jwtExport.ResponseType = jwt.ResponseType(export.ResponseType.ValueString())
			}
			if !export.ResponseThreshold.IsNull() && !export.ResponseThreshold.IsUnknown() {
				duration, diags := export.ResponseThreshold.ValueGoDuration()
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
				jwtExport.ResponseThreshold = duration
			}
			if !export.AccountTokenPosition.IsNull() {
				jwtExport.AccountTokenPosition = uint(export.AccountTokenPosition.ValueInt64())
			}
			if !export.Advertise.IsNull() {
				jwtExport.Advertise = export.Advertise.ValueBool()
			}
			if !export.AllowTrace.IsNull() {
				jwtExport.AllowTrace = export.AllowTrace.ValueBool()
			}
			if !export.Description.IsNull() {
				jwtExport.Description = export.Description.ValueString()
			}
			if !export.InfoURL.IsNull() {
				jwtExport.InfoURL = export.InfoURL.ValueString()
			}

			accountClaims.Exports.Add(jwtExport)
		}
	}

	// Handle imports
	if !data.Imports.IsNull() && len(data.Imports.Elements()) > 0 {
		var imports []ImportModel
		resp.Diagnostics.Append(data.Imports.ElementsAs(ctx, &imports, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, imp := range imports {
			jwtImport := &jwt.Import{
				Subject: jwt.Subject(imp.Subject.ValueString()),
				Account: imp.Account.ValueString(),
			}

			// Set import type
			switch imp.Type.ValueString() {
			case "stream":
				jwtImport.Type = jwt.Stream
			case "service":
				jwtImport.Type = jwt.Service
			default:
				resp.Diagnostics.AddError(
					"Invalid import type",
					fmt.Sprintf("Import type must be 'stream' or 'service', got: %s", imp.Type.ValueString()),
				)
				return
			}

			// Optional fields
			if !imp.Name.IsNull() {
				jwtImport.Name = imp.Name.ValueString()
			}
			if !imp.Token.IsNull() {
				jwtImport.Token = imp.Token.ValueString()
			}
			if !imp.LocalSubject.IsNull() {
				jwtImport.LocalSubject = jwt.RenamingSubject(imp.LocalSubject.ValueString())
			}
			if !imp.Share.IsNull() {
				jwtImport.Share = imp.Share.ValueBool()
			}
			if !imp.AllowTrace.IsNull() {
				jwtImport.AllowTrace = imp.AllowTrace.ValueBool()
			}

			accountClaims.Imports.Add(jwtImport)
		}
	}

	// Add signing keys if provided
	if !data.SigningKeys.IsNull() && !data.SigningKeys.IsUnknown() {
		var signingKeys []string
		resp.Diagnostics.Append(data.SigningKeys.ElementsAs(ctx, &signingKeys, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, key := range signingKeys {
			if !strings.HasPrefix(key, "A") {
				resp.Diagnostics.AddError(
					"Invalid signing key",
					fmt.Sprintf("Signing keys must be account public keys (start with 'A'), got: %s", key),
				)
				return
			}
			accountClaims.SigningKeys.Add(key)
		}
	}

	// Sign the JWT with operator key (already have operatorKP from above)
	accountJWT, err := accountClaims.Encode(operatorKP)
	if err != nil {
		resp.Diagnostics.AddError("Failed to encode account JWT", err.Error())
		return
	}

	// Update JWT while preserving immutable fields
	data.ID = state.ID
	data.PublicKey = state.PublicKey
	data.Subject = state.Subject
	data.IssuerSeed = state.IssuerSeed
	data.JWT = types.StringValue(accountJWT)

	tflog.Trace(ctx, "updated account resource")

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
