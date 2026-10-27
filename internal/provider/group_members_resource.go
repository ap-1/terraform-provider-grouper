package provider

import (
	"context"
	"fmt"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure   = &groupMembersResource{}
	_ resource.ResourceWithImportState = &groupMembersResource{}
)

type groupMembersResource struct {
	client *grouper.Client
}

type groupMembersResourceModel struct {
	Group    types.String `tfsdk:"group"`
	Subjects types.Map    `tfsdk:"subjects"`
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
	emptyMap := mapdefault.StaticValue(types.MapValueMust(subjectsType, map[string]attr.Value{}))

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
			"subjects": schema.MapAttribute{
				MarkdownDescription: "Subject IDs of people in the group, keyed by subject source, such as `{ ldap = [\"charlie\", \"lucy\"] }`.",
				ElementType:         subjectsType,
				Optional:            true,
				Computed:            true,
				Default:             emptyMap,
				Validators: []validator.Map{
					mapvalidator.KeysAre(
						stringvalidator.LengthAtLeast(1),
						stringvalidator.NoneOf(grouper.GroupSource),
					),
					// An empty list would read back as a missing key
					mapvalidator.ValueSetsAre(setvalidator.SizeAtLeast(1)),
				},
			},
			"groups": schema.SetAttribute{
				MarkdownDescription: "UUIDs of groups nested in the group, whose members become members of this group too. Use this rather than `subjects` for groups.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             emptySet,
			},
		},
	}
}

func (r *groupMembersResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
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

	members, err := r.client.GroupMembers(ctx, state.Group.ValueString(), grouper.ImmediateMembers)

	// The group was deleted, taking its members with it
	if hasErrorCode(err, "GROUP_NOT_FOUND") {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group members", err.Error())
		return
	}

	state.Subjects, state.Groups = splitMembers(members)
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

	err := r.client.SetGroupMembers(ctx, state.Group.ValueString(), nil)
	if err != nil && !hasErrorCode(err, "GROUP_NOT_FOUND") {
		resp.Diagnostics.AddError("Unable to remove group members", err.Error())
	}
}

// Imports by the group's current full name
func (r *groupMembersResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	group, err := r.client.FindGroup(ctx, req.ID)
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
	var subjects map[string][]string
	var groups []string
	if diags := plan.Subjects.ElementsAs(ctx, &subjects, false); diags.HasError() {
		addError("Invalid subjects", fmt.Sprint(diags))
		return
	}
	if diags := plan.Groups.ElementsAs(ctx, &groups, false); diags.HasError() {
		addError("Invalid groups", fmt.Sprint(diags))
		return
	}

	members := make([]grouper.Subject, 0, len(groups))
	for source, ids := range subjects {
		for _, id := range ids {
			members = append(members, grouper.Subject{ID: id, SourceID: source})
		}
	}
	for _, id := range groups {
		members = append(members, grouper.Subject{ID: id, SourceID: grouper.GroupSource})
	}

	if err := r.client.SetGroupMembers(ctx, plan.Group.ValueString(), members); err != nil {
		addError("Unable to set group members", err.Error())
	}
}

func stringSet(values []string) types.Set {
	elements := make([]attr.Value, len(values))
	for i, v := range values {
		elements[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elements)
}

// Members as subjects keyed by source, and the UUIDs of member groups
func splitMembers(members []grouper.Subject) (subjects types.Map, groups types.Set) {
	bySource := map[string][]string{}
	var groupIDs []string
	for _, member := range members {
		if member.SourceID == grouper.GroupSource {
			groupIDs = append(groupIDs, member.ID)
		} else {
			bySource[member.SourceID] = append(bySource[member.SourceID], member.ID)
		}
	}

	elements := make(map[string]attr.Value, len(bySource))
	for source, ids := range bySource {
		elements[source] = stringSet(ids)
	}
	return types.MapValueMust(subjectsType, elements), stringSet(groupIDs)
}

// Element type of subjects, a set of subject IDs
var subjectsType = types.SetType{ElemType: types.StringType}
