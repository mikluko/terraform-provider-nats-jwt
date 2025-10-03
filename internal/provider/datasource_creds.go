package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CredsDataSource{}

func NewCredsDataSource() datasource.DataSource {
	return &CredsDataSource{}
}

type CredsDataSource struct{}

type CredsDataSourceModel struct {
	ID    types.String `tfsdk:"id"`
	JWT   types.String `tfsdk:"jwt"`
	Seed  types.String `tfsdk:"seed"`
	Creds types.String `tfsdk:"creds"`
}

func (d *CredsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_creds"
}

func (d *CredsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Generates NATS credentials file content from a JWT and seed. Use with nsc_user resource outputs.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Internal identifier",
			},
			"jwt": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User JWT token",
			},
			"seed": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "User seed (private key)",
			},
			"creds": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Credentials file content in NATS format",
			},
		},
	}
}

func (d *CredsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CredsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	jwt := data.JWT.ValueString()
	seed := data.Seed.ValueString()

	// Generate creds file content
	creds := fmt.Sprintf(`-----BEGIN NATS USER JWT-----
%s
------END NATS USER JWT------

************************* IMPORTANT *************************
NKEY Seed printed below can be used to sign and prove identity.
NKEYs are sensitive and should be treated as secrets.

-----BEGIN USER NKEY SEED-----
%s
------END USER NKEY SEED------

*************************************************************
`, jwt, seed)

	data.ID = types.StringValue(jwt)
	data.Creds = types.StringValue(creds)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
