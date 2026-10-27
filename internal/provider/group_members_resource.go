package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure   = &groupMembersResource{}
	_ resource.ResourceWithImportState = &groupMembersResource{}
)

type groupMembersResource struct {
	provider *providerData
}

type groupMembersResourceModel struct {
	Group    types.String `tfsdk:"group"`
	Subjects types.Set    `tfsdk:"subjects"`
	Groups   types.Set    `tfsdk:"groups"`
}

func NewGroupMembersResource() resource.Resource {
	return &groupMembersResource{}
}

func (r *groupMembersResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_members"
}

func (r *groupMembersResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	emptySet := setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{}))

	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the immediate members of a Grouper group, removing anyone else. Members the group has only through nested groups aren't listed. Use one of these per group.",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				MarkdownDescription: "UUID of the group, such as `grouper_group.example.id`.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subjects": schema.SetAttribute{
				MarkdownDescription: "Subject IDs of people in the group, from the provider's `subject_source`.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             emptySet,
			},
			"groups": schema.SetAttribute{
				MarkdownDescription: "UUIDs of groups nested in the group, whose members become members of this group too.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             emptySet,
			},
		},
	}
}

func (r *groupMembersResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.provider = configuredProvider(req.ProviderData, &resp.Diagnostics)
}

func (r *groupMembersResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupMembersResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.set(ctx, &plan, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupMembersResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupMembersResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, err := r.provider.client.GroupMembers(ctx, state.Group.ValueString())

	// The group was deleted, taking its members with it
	if isGroupNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group members", err.Error())
		return
	}

	var subjects, groups []string
	for _, member := range members {
		if member.SourceID == grouper.GroupSource {
			groups = append(groups, member.ID)
		} else {
			subjects = append(subjects, member.ID)
		}
	}

	state.Subjects = stringSet(subjects)
	state.Groups = stringSet(groups)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *groupMembersResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupMembersResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.set(ctx, &plan, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupMembersResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupMembersResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.provider.client.SetGroupMembers(ctx, state.Group.ValueString(), nil)
	if err != nil && !isGroupNotFound(err) {
		resp.Diagnostics.AddError("Unable to remove group members", err.Error())
	}
}

// Imports by the group's current full name
func (r *groupMembersResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	group, err := r.provider.client.FindGroup(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import group members", err.Error())
		return
	}
	if group == nil {
		resp.Diagnostics.AddError("Group not found", fmt.Sprintf("No group is currently named %q.", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group"), group.UUID)...)
}

// Replaces the group's members with the planned ones
func (r *groupMembersResource) set(ctx context.Context, plan *groupMembersResourceModel, addError func(string, string)) {
	var subjects, groups []string
	if diags := plan.Subjects.ElementsAs(ctx, &subjects, false); diags.HasError() {
		addError("Invalid subjects", fmt.Sprint(diags))
		return
	}
	if diags := plan.Groups.ElementsAs(ctx, &groups, false); diags.HasError() {
		addError("Invalid groups", fmt.Sprint(diags))
		return
	}

	members := make([]grouper.Subject, 0, len(subjects)+len(groups))
	for _, id := range subjects {
		members = append(members, grouper.Subject{ID: id, SourceID: r.provider.subjectSource})
	}
	for _, id := range groups {
		members = append(members, grouper.Subject{ID: id, SourceID: grouper.GroupSource})
	}

	if err := r.provider.client.SetGroupMembers(ctx, plan.Group.ValueString(), members); err != nil {
		addError("Unable to set group members", err.Error())
	}
}

func isGroupNotFound(err error) bool {
	var grouperErr *grouper.Error
	return errors.As(err, &grouperErr) && grouperErr.Code == "GROUP_NOT_FOUND"
}

func stringSet(values []string) types.Set {
	elements := make([]attr.Value, len(values))
	for i, v := range values {
		elements[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elements)
}
