package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &NATSJWTProvider{}
var _ provider.ProviderWithFunctions = &NATSJWTProvider{}

type NATSJWTProvider struct {
	version string
}

type NATSJWTProviderModel struct{}

func (p *NATSJWTProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "natsjwt"
	resp.Version = p.version
}

func (p *NATSJWTProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Provider for managing NATS JWT tokens. All keys and JWTs are stored in Terraform state.`,
	}
}

func (p *NATSJWTProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data NATSJWTProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (p *NATSJWTProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewOperatorResource,
		NewAccountResource,
		NewUserResource,
	}
}

func (p *NATSJWTProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *NATSJWTProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NATSJWTProvider{
			version: version,
		}
	}
}
