package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.ResourceWithConfigure      = &groupMembersResource{}
	_ resource.ResourceWithImportState    = &groupMembersResource{}
	_ resource.ResourceWithValidateConfig = &groupMembersResource{}
)

type groupMembersResource struct {
	client *grouper.Client
}

type groupMembersResourceModel struct {
	Group     types.String `tfsdk:"group"`
	Subjects  types.Map    `tfsdk:"subjects"`
	Groups    types.Set    `tfsdk:"groups"`
	Scheduled types.Set    `tfsdk:"scheduled"`
}

type scheduledModel struct {
	Subject types.String `tfsdk:"subject"`
	Source  types.String `tfsdk:"source"`
	Group   types.String `tfsdk:"group"`
	Starts  types.String `tfsdk:"starts"`
	Ends    types.String `tfsdk:"ends"`
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
	emptyScheduled := setdefault.StaticValue(types.SetValueMust(scheduledType, []attr.Value{}))
	sibling := func(name string) path.Expression { return path.MatchRelative().AtParent().AtName(name) }

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
			"scheduled": schema.SetNestedAttribute{
				MarkdownDescription: "Members that Grouper adds on `starts` and removes on `ends`, without OpenTofu running. A member that has ended stays listed until it's removed from this set. Needs the provider's `time_zone`.",
				Optional:            true,
				Computed:            true,
				Default:             emptyScheduled,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"subject": schema.StringAttribute{
							MarkdownDescription: "Subject ID of a person. Exactly one of `subject` and `group` is required.",
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(sibling("group")),
								stringvalidator.AlsoRequires(sibling("source")),
							},
						},
						"source": schema.StringAttribute{
							MarkdownDescription: "Subject source of `subject`, such as `ldap`.",
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.AlsoRequires(sibling("subject")),
								stringvalidator.LengthAtLeast(1),
								stringvalidator.NoneOf(grouper.GroupSource),
							},
						},
						"group": schema.StringAttribute{
							MarkdownDescription: "UUID of a group to nest.",
							Optional:            true,
						},
						"starts": schema.StringAttribute{
							MarkdownDescription: "When the membership starts, as an RFC 3339 time such as `2026-08-20T00:00:00-04:00`. At least one of `starts` and `ends` is required.",
							Optional:            true,
							Validators: []validator.String{
								timeValidator{},
								stringvalidator.AtLeastOneOf(sibling("ends")),
							},
						},
						"ends": schema.StringAttribute{
							MarkdownDescription: "When the membership ends, as an RFC 3339 time.",
							Optional:            true,
							Validators: []validator.String{
								timeValidator{},
							},
						},
					},
				},
			},
		},
	}
}

// Rejects duplicate members and scheduled members that end before they start
func (r *groupMembersResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config groupMembersResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	seen := map[grouper.Subject]bool{}
	add := func(attribute string, member grouper.Subject) {
		if seen[member] {
			resp.Diagnostics.AddAttributeError(path.Root(attribute), "Duplicate member", fmt.Sprintf("%s is listed more than once.", member.ID))
		}
		seen[member] = true
	}

	var subjects map[string][]types.String
	if !config.Subjects.IsUnknown() {
		resp.Diagnostics.Append(config.Subjects.ElementsAs(ctx, &subjects, true)...)
	}
	for source, ids := range subjects {
		for _, id := range ids {
			if !id.IsUnknown() {
				add("subjects", grouper.Subject{ID: id.ValueString(), SourceID: source})
			}
		}
	}

	var groups []types.String
	if !config.Groups.IsUnknown() {
		resp.Diagnostics.Append(config.Groups.ElementsAs(ctx, &groups, true)...)
	}
	for _, id := range groups {
		if !id.IsUnknown() {
			add("groups", grouper.Subject{ID: id.ValueString(), SourceID: grouper.GroupSource})
		}
	}

	var scheduled []scheduledModel
	if !config.Scheduled.IsUnknown() {
		resp.Diagnostics.Append(config.Scheduled.ElementsAs(ctx, &scheduled, true)...)
	}
	for _, s := range scheduled {
		if !s.Subject.IsUnknown() && !s.Source.IsUnknown() && !s.Group.IsUnknown() {
			add("scheduled", s.member())
		}

		starts, startsErr := parseTime(s.Starts)
		ends, endsErr := parseTime(s.Ends)
		if startsErr == nil && endsErr == nil && !starts.IsZero() && !ends.IsZero() && !ends.After(starts) {
			resp.Diagnostics.AddAttributeError(path.Root("scheduled"), "Membership ends before it starts", fmt.Sprintf("%s ends at %s, before it starts at %s.", s.member().ID, s.Ends.ValueString(), s.Starts.ValueString()))
		}
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

	r.set(ctx, &plan, &resp.Diagnostics)
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

	memberships, err := r.client.GroupMemberships(ctx, state.Group.ValueString())

	// The group was deleted, taking its members with it
	if hasErrorCode(err, "GROUP_NOT_FOUND") {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group members", err.Error())
		return
	}

	// Keeps the configured time strings when Grouper's dates match them
	var prior []scheduledModel
	resp.Diagnostics.Append(state.Scheduled.ElementsAs(ctx, &prior, false)...)
	priorByMember := make(map[grouper.Subject]scheduledModel, len(prior))
	for _, s := range prior {
		priorByMember[s.member()] = s
	}

	var undated []grouper.Subject
	// Empty rather than nil, which would read back as null
	scheduled := []scheduledModel{}
	for _, m := range memberships {
		if m.Starts.IsZero() && m.Ends.IsZero() {
			undated = append(undated, m.Subject)
			continue
		}

		s := scheduledModel{
			Subject: types.StringNull(),
			Source:  types.StringNull(),
			Group:   types.StringNull(),
			Starts:  formatTime(m.Starts),
			Ends:    formatTime(m.Ends),
		}
		if m.SourceID == grouper.GroupSource {
			s.Group = types.StringValue(m.ID)
		} else {
			s.Subject = types.StringValue(m.ID)
			s.Source = types.StringValue(m.SourceID)
		}
		if p, ok := priorByMember[m.Subject]; ok {
			s.Starts = sameInstant(p.Starts, m.Starts, s.Starts)
			s.Ends = sameInstant(p.Ends, m.Ends, s.Ends)
		}
		scheduled = append(scheduled, s)
	}

	state.Subjects, state.Groups = splitMembers(undated)
	var diags diag.Diagnostics
	state.Scheduled, diags = types.SetValueFrom(ctx, scheduledType, scheduled)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *groupMembersResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupMembersResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.set(ctx, &plan, &resp.Diagnostics)
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

	err := r.client.SetGroupMemberships(ctx, state.Group.ValueString(), nil)
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
func (r *groupMembersResource) set(ctx context.Context, plan *groupMembersResourceModel, diags *diag.Diagnostics) {
	var subjects map[string][]string
	var groups []string
	var scheduled []scheduledModel
	diags.Append(plan.Subjects.ElementsAs(ctx, &subjects, false)...)
	diags.Append(plan.Groups.ElementsAs(ctx, &groups, false)...)
	diags.Append(plan.Scheduled.ElementsAs(ctx, &scheduled, false)...)
	if diags.HasError() {
		return
	}

	memberships := make([]grouper.Membership, 0, len(groups)+len(scheduled))
	for source, ids := range subjects {
		for _, id := range ids {
			memberships = append(memberships, grouper.Membership{Subject: grouper.Subject{ID: id, SourceID: source}})
		}
	}
	for _, id := range groups {
		memberships = append(memberships, grouper.Membership{Subject: grouper.Subject{ID: id, SourceID: grouper.GroupSource}})
	}
	for _, s := range scheduled {
		// Already checked by timeValidator
		starts, _ := parseTime(s.Starts)
		ends, _ := parseTime(s.Ends)
		memberships = append(memberships, grouper.Membership{Subject: s.member(), Starts: starts, Ends: ends})
	}

	if err := r.client.SetGroupMemberships(ctx, plan.Group.ValueString(), memberships); err != nil {
		diags.AddError("Unable to set group members", err.Error())
	}
}

func (s scheduledModel) member() grouper.Subject {
	if !s.Group.IsNull() {
		return grouper.Subject{ID: s.Group.ValueString(), SourceID: grouper.GroupSource}
	}
	return grouper.Subject{ID: s.Subject.ValueString(), SourceID: s.Source.ValueString()}
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

// RFC 3339 time, or the zero time when unset
func parseTime(v types.String) (time.Time, error) {
	if v.IsNull() || v.IsUnknown() {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, v.ValueString())
}

// t in RFC 3339, or null for the zero time
func formatTime(t time.Time) types.String {
	if t.IsZero() {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339Nano))
}

// prior if it's the same instant as t, otherwise read
func sameInstant(prior types.String, t time.Time, read types.String) types.String {
	if p, err := parseTime(prior); err == nil && p.Equal(t) {
		return prior
	}
	return read
}

// Element type of subjects, a set of subject IDs
var subjectsType = types.SetType{ElemType: types.StringType}

var scheduledType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"subject": types.StringType,
	"source":  types.StringType,
	"group":   types.StringType,
	"starts":  types.StringType,
	"ends":    types.StringType,
}}

// Rejects anything but RFC 3339 with at most millisecond precision
type timeValidator struct{}

func (v timeValidator) Description(context.Context) string {
	return "must be an RFC 3339 time with at most millisecond precision"
}

func (v timeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v timeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	t, err := parseTime(req.ConfigValue)
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid time", fmt.Sprintf("%q isn't an RFC 3339 time, such as 2026-08-20T00:00:00-04:00.", req.ConfigValue.ValueString()))
		return
	}
	if !t.Equal(t.Truncate(time.Millisecond)) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid time", fmt.Sprintf("%q is more precise than Grouper's milliseconds.", req.ConfigValue.ValueString()))
	}
}
