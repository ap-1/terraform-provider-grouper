package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

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
	URL           types.String `tfsdk:"url"`
	Username      types.String `tfsdk:"username"`
	Password      types.String `tfsdk:"password"`
	SubjectSource types.String `tfsdk:"subject_source"`
}

// What the provider hands its resources and data sources
type providerData struct {
	client *grouper.Client
	// Source of people added as members, or empty to search every source
	subjectSource string
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
			"subject_source": schema.StringAttribute{
				MarkdownDescription: "Subject source of the people in `grouper_group_members`, such as `ldap`. Defaults to `GROUPER_SUBJECT_SOURCE`, and when neither is set, Grouper searches every source for each person.",
				Optional:            true,
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
	if model.SubjectSource.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("subject_source"), "Unknown Grouper subject_source", "The provider can't add members until subject_source is known.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	subjectSource := model.SubjectSource.ValueString()
	if model.SubjectSource.IsNull() {
		subjectSource = os.Getenv("GROUPER_SUBJECT_SOURCE")
	}

	data := &providerData{
		client:        grouper.NewClient(url, username, password),
		subjectSource: subjectSource,
	}
	resp.DataSourceData = data
	resp.ResourceData = data
}

func (p *grouperProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFolderResource,
		NewGroupResource,
		NewGroupMembersResource,
		NewGroupPrivilegesResource,
		NewFolderPrivilegesResource,
	}
}

func (p *grouperProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewFolderDataSource,
	}
}

// Whether err is a Grouper error with one of codes, such as GROUP_NOT_FOUND
func hasErrorCode(err error, codes ...string) bool {
	var grouperErr *grouper.Error
	return errors.As(err, &grouperErr) && slices.Contains(codes, grouperErr.Code)
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

// Provider data from a resource or data source's Configure request, nil until the provider is configured
func configuredProvider(data any, diags *diag.Diagnostics) *providerData {
	if data == nil {
		return nil
	}

	provider, ok := data.(*providerData)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *providerData, got %T.", data))
	}
	return provider
}
