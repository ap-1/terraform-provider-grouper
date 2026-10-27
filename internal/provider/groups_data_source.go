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

var _ datasource.DataSourceWithConfigure = &groupsDataSource{}

type groupsDataSource struct {
	client *grouper.Client
}

type groupsDataSourceModel struct {
	Folder    types.String           `tfsdk:"folder"`
	Recursive types.Bool             `tfsdk:"recursive"`
	Groups    []groupsDataSourceItem `tfsdk:"groups"`
}

type groupsDataSourceItem struct {
	ID               types.String    `tfsdk:"id"`
	Name             types.String    `tfsdk:"name"`
	DisplayExtension types.String    `tfsdk:"display_extension"`
	DisplayName      types.String    `tfsdk:"display_name"`
	Description      types.String    `tfsdk:"description"`
	Composite        *compositeModel `tfsdk:"composite"`
}

func NewGroupsDataSource() datasource.DataSource {
	return &groupsDataSource{}
}

func (d *groupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_groups"
}

func (d *groupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the groups in a Grouper folder.",
		Attributes: map[string]schema.Attribute{
			"folder": schema.StringAttribute{
				MarkdownDescription: "Current full name of the folder, such as `org:dept:apps`.",
				Required:            true,
			},
			"recursive": schema.BoolAttribute{
				MarkdownDescription: "Also lists the groups in the folder's subfolders, at any depth. Defaults to `false`.",
				Optional:            true,
			},
			"groups": schema.ListNestedAttribute{
				MarkdownDescription: "The groups, sorted by full name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "UUID of the group.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Full name of the group.",
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
				},
			},
		},
	}
}

func (d *groupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *groupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model groupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	folder := model.Folder.ValueString()

	// Grouper lists no groups for a missing folder, which would hide a typo
	stem, err := d.client.FindStem(ctx, folder)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read folder", err.Error())
		return
	}
	if stem == nil {
		resp.Diagnostics.AddAttributeError(path.Root("folder"), "Folder not found", fmt.Sprintf("No folder is currently named %q.", folder))
		return
	}

	groups, err := d.client.ListGroups(ctx, folder, model.Recursive.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list groups", err.Error())
		return
	}

	model.Groups = make([]groupsDataSourceItem, len(groups))
	for i, group := range groups {
		model.Groups[i] = groupsDataSourceItem{
			ID:               types.StringValue(group.UUID),
			Name:             types.StringValue(group.Name),
			DisplayExtension: types.StringValue(group.DisplayExtension),
			DisplayName:      types.StringValue(group.DisplayName),
			Description:      types.StringValue(group.Description),
			Composite:        compositeFrom(group.Composite),
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
