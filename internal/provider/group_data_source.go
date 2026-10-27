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

var _ datasource.DataSourceWithConfigure = &groupDataSource{}

type groupDataSource struct {
	client *grouper.Client
}

type groupDataSourceModel struct {
	Name             types.String    `tfsdk:"name"`
	ID               types.String    `tfsdk:"id"`
	DisplayExtension types.String    `tfsdk:"display_extension"`
	DisplayName      types.String    `tfsdk:"display_name"`
	Description      types.String    `tfsdk:"description"`
	Composite        *compositeModel `tfsdk:"composite"`
}

func NewGroupDataSource() datasource.DataSource {
	return &groupDataSource{}
}

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Grouper group by its full name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Current full name of the group, with segments separated by colons, such as `org:dept:apps:admins`. Grouper keeps a renamed group's old names as aliases, but they don't match here.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the group.",
				Computed:            true,
			},
			"display_extension": schema.StringAttribute{
				MarkdownDescription: "Display name of the group itself, shown in the Grouper UI.",
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the group and its folders, separated by colons.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the group.",
				Computed:            true,
			},
			"composite": compositeDataSourceAttribute(),
		},
	}
}

// Read-only composite settings, shared by the group data sources
func compositeDataSourceAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Set when the group's members are the combination of two other groups' members.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: "`union`, `intersection`, or `complement`, which is members of `left_group` who aren't in `right_group`.",
				Computed:            true,
			},
			"left_group": schema.StringAttribute{
				MarkdownDescription: "UUID of the first group.",
				Computed:            true,
			},
			"right_group": schema.StringAttribute{
				MarkdownDescription: "UUID of the second group.",
				Computed:            true,
			},
		},
	}
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model groupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := d.client.FindGroup(ctx, model.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group", err.Error())
		return
	}
	if group == nil {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Group not found", fmt.Sprintf("No group is currently named %q.", model.Name.ValueString()))
		return
	}

	model.ID = types.StringValue(group.UUID)
	model.DisplayExtension = types.StringValue(group.DisplayExtension)
	model.DisplayName = types.StringValue(group.DisplayName)
	model.Description = types.StringValue(group.Description)
	model.Composite = compositeFrom(group.Composite)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
