package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &grouperProvider{}

type grouperProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &grouperProvider{version: version}
	}
}

func (p *grouperProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "grouper"
	resp.Version = p.version
}

func (p *grouperProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{}
}

func (p *grouperProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *grouperProvider) Resources(context.Context) []func() resource.Resource {
	return nil
}

func (p *grouperProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}
