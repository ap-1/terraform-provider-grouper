package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &grouperProvider{}

type grouperProvider struct {
	version string
}

type grouperProviderModel struct {
	URL      types.String `tfsdk:"url"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// Connection settings resolved from the provider block and environment
type config struct {
	URL      string
	Username string
	Password string
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
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages folders, groups, memberships, and privileges in Internet2 Grouper through its web services.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "Grouper web services REST endpoint, such as `https://grouper.example.edu/grouper-ws/servicesRest/v2_6_000`. Defaults to `GROUPER_URL`.",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for basic auth. Defaults to `GROUPER_USERNAME`.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for basic auth. Defaults to `GROUPER_PASSWORD`.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *grouperProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var model grouperProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := config{
		URL:      resolve(resp, "url", model.URL, "GROUPER_URL"),
		Username: resolve(resp, "username", model.Username, "GROUPER_USERNAME"),
		Password: resolve(resp, "password", model.Password, "GROUPER_PASSWORD"),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.DataSourceData = cfg
	resp.ResourceData = cfg
}

func (p *grouperProvider) Resources(context.Context) []func() resource.Resource {
	return nil
}

func (p *grouperProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

// Attribute value, or its environment variable when the attribute is unset
func resolve(resp *provider.ConfigureResponse, name string, value types.String, env string) string {
	if value.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown Grouper "+name, "The provider can't connect until "+name+" is known.")
		return ""
	}

	if !value.IsNull() {
		return value.ValueString()
	}

	if v := os.Getenv(env); v != "" {
		return v
	}

	resp.Diagnostics.AddAttributeError(path.Root(name), "Missing Grouper "+name, "Set "+name+" in the provider block or "+env+" in the environment.")
	return ""
}
