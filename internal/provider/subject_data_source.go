package provider

import (
	"context"
	"fmt"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &subjectDataSource{}

type subjectDataSource struct {
	client *grouper.Client
}

type subjectDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Source      types.String `tfsdk:"source"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func NewSubjectDataSource() datasource.DataSource {
	return &subjectDataSource{}
}

func (d *subjectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subject"
}

func (d *subjectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a person or other subject, failing if it doesn't exist.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Subject ID, such as `charlie`.",
				Required:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "Subject source, such as `ldap`.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the subject, such as a person's full name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the subject, which depends on the source.",
				Computed:            true,
			},
		},
	}
}

func (d *subjectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *subjectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model subjectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subject, err := d.client.FindSubject(ctx, grouper.Subject{ID: model.ID.ValueString(), SourceID: model.Source.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read subject", err.Error())
		return
	}
	if subject == nil {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Subject not found", fmt.Sprintf("Source %q has no subject %q.", model.Source.ValueString(), model.ID.ValueString()))
		return
	}

	model.Name = types.StringValue(subject.Name)
	model.Description = types.StringValue(subject.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
