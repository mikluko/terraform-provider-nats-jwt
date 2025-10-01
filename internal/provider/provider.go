package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &NSCProvider{}
var _ provider.ProviderWithFunctions = &NSCProvider{}

type NSCProvider struct {
	version string
}

type NSCProviderModel struct{}

func (p *NSCProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "nsc"
	resp.Version = p.version
}

func (p *NSCProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Provider for managing NATS JWT tokens. All keys and JWTs are stored in Terraform state.`,
	}
}

func (p *NSCProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data NSCProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (p *NSCProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNKeyResource,
		NewOperatorResource,
		NewAccountResource,
		NewUserResource,
	}
}

func (p *NSCProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCredsDataSource,
	}
}

func (p *NSCProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &NSCProvider{
			version: version,
		}
	}
}
