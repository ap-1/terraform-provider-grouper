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

var _ datasource.DataSourceWithConfigure = &folderDataSource{}

type folderDataSource struct {
	client *grouper.Client
}

type folderDataSourceModel struct {
	Name             types.String `tfsdk:"name"`
	ID               types.String `tfsdk:"id"`
	DisplayExtension types.String `tfsdk:"display_extension"`
	DisplayName      types.String `tfsdk:"display_name"`
	Description      types.String `tfsdk:"description"`
}

func NewFolderDataSource() datasource.DataSource {
	return &folderDataSource{}
}

func (d *folderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (d *folderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Grouper folder by its full name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Full name of the folder, with segments separated by colons, such as `org:dept:apps`.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the folder.",
				Computed:            true,
			},
			"display_extension": schema.StringAttribute{
				MarkdownDescription: "Display name of the folder itself, shown in the Grouper UI.",
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the folder and its parents, separated by colons.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the folder.",
				Computed:            true,
			},
		},
	}
}

func (d *folderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = providerClient(req.ProviderData, &resp.Diagnostics)
}

func (d *folderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model folderDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stem, err := d.client.FindStem(ctx, model.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read folder", err.Error())
		return
	}
	if stem == nil {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Folder not found", fmt.Sprintf("No folder is named %q.", model.Name.ValueString()))
		return
	}

	model.ID = types.StringValue(stem.UUID)
	model.DisplayExtension = types.StringValue(stem.DisplayExtension)
	model.DisplayName = types.StringValue(stem.DisplayName)
	model.Description = types.StringValue(stem.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
