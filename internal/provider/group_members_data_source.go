package provider

import (
	"context"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &groupMembersDataSource{}

type groupMembersDataSource struct {
	client *grouper.Client
}

type groupMembersDataSourceModel struct {
	Group     types.String `tfsdk:"group"`
	Immediate types.Bool   `tfsdk:"immediate"`
	Subjects  types.Map    `tfsdk:"subjects"`
	Groups    types.Set    `tfsdk:"groups"`
}

func NewGroupMembersDataSource() datasource.DataSource {
	return &groupMembersDataSource{}
}

func (d *groupMembersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_members"
}

func (d *groupMembersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the members of a Grouper group, including those it has through nested groups unless `immediate` is set.",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				MarkdownDescription: "UUID of the group, such as `data.grouper_group.example.id`.",
				Required:            true,
			},
			"immediate": schema.BoolAttribute{
				MarkdownDescription: "Lists only the members added to the group directly. Defaults to `false`.",
				Optional:            true,
			},
			"subjects": schema.MapAttribute{
				MarkdownDescription: "Subject IDs of the group's members, keyed by subject source, such as `{ ldap = [\"charlie\", \"lucy\"] }`.",
				ElementType:         subjectsType,
				Computed:            true,
			},
			"groups": schema.SetAttribute{
				MarkdownDescription: "UUIDs of the groups nested in the group, at any depth unless `immediate` is set.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *groupMembersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *groupMembersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model groupMembersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := grouper.AllMembers
	if model.Immediate.ValueBool() {
		filter = grouper.ImmediateMembers
	}

	members, err := d.client.GroupMembers(ctx, model.Group.ValueString(), filter)
	if hasErrorCode(err, "GROUP_NOT_FOUND") {
		resp.Diagnostics.AddAttributeError(path.Root("group"), "Group not found", "No group has UUID "+model.Group.ValueString()+".")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group members", err.Error())
		return
	}

	model.Subjects, model.Groups = splitMembers(members)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
