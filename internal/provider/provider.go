package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

	url := resolve(resp, "url", model.URL, "GROUPER_URL")
	username := resolve(resp, "username", model.Username, "GROUPER_USERNAME")
	password := resolve(resp, "password", model.Password, "GROUPER_PASSWORD")
	if resp.Diagnostics.HasError() {
		return
	}

	client := grouper.NewClient(url, username, password)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *grouperProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFolderResource,
	}
}

func (p *grouperProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewFolderDataSource,
	}
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

// Client from the data the provider hands its resources and data sources, nil until the provider is configured
func providerClient(data any, diags *diag.Diagnostics) *grouper.Client {
	if data == nil {
		return nil
	}

	client, ok := data.(*grouper.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *grouper.Client, got %T.", data))
	}
	return client
}
