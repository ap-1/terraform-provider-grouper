package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure        = &privilegesResource{}
	_ resource.ResourceWithConfigValidators = &privilegesResource{}
	_ resource.ResourceWithImportState      = &privilegesResource{}
)

// What differs between grouper_group_privileges and grouper_folder_privileges
type privilegeKind struct {
	// group or folder, which also names the target attribute
	target string
	names  []string
	// Grouper's error code when the target doesn't exist
	notFoundCode string
	find         func(client *grouper.Client, ctx context.Context, name string) (uuid string, err error)
	get          func(client *grouper.Client, ctx context.Context, uuid string, subject grouper.Subject) ([]string, error)
	set          func(client *grouper.Client, ctx context.Context, uuid string, subject grouper.Subject, privileges []string) error
}

var groupPrivilegeKind = privilegeKind{
	target:       "group",
	names:        grouper.GroupPrivilegeNames,
	notFoundCode: "GROUP_NOT_FOUND",
	find: func(client *grouper.Client, ctx context.Context, name string) (string, error) {
		group, err := client.FindGroup(ctx, name)
		if err != nil || group == nil {
			return "", err
		}
		return group.UUID, nil
	},
	get: (*grouper.Client).GroupPrivileges,
	set: (*grouper.Client).SetGroupPrivileges,
}

var folderPrivilegeKind = privilegeKind{
	target:       "folder",
	names:        grouper.StemPrivilegeNames,
	notFoundCode: "STEM_NOT_FOUND",
	find: func(client *grouper.Client, ctx context.Context, name string) (string, error) {
		stem, err := client.FindStem(ctx, name)
		if err != nil || stem == nil {
			return "", err
		}
		return stem.UUID, nil
	},
	get: (*grouper.Client).StemPrivileges,
	set: (*grouper.Client).SetStemPrivileges,
}

type privilegesResource struct {
	kind     privilegeKind
	provider *providerData
}

// Held by path rather than struct tags, since the target attribute is named group or folder
type privilegesResourceModel struct {
	Target       types.String
	Subject      types.String
	SubjectGroup types.String
	Privileges   types.Set
}

type attributeGetter interface {
	GetAttribute(context.Context, path.Path, any) diag.Diagnostics
}

type attributeSetter interface {
	SetAttribute(context.Context, path.Path, any) diag.Diagnostics
}

func (r *privilegesResource) fields(m *privilegesResourceModel) map[string]any {
	return map[string]any{
		r.kind.target:   &m.Target,
		"subject":       &m.Subject,
		"subject_group": &m.SubjectGroup,
		"privileges":    &m.Privileges,
	}
}

func (r *privilegesResource) get(ctx context.Context, from attributeGetter, diags *diag.Diagnostics) privilegesResourceModel {
	var m privilegesResourceModel
	for name, field := range r.fields(&m) {
		diags.Append(from.GetAttribute(ctx, path.Root(name), field)...)
	}
	return m
}

func (r *privilegesResource) put(ctx context.Context, to attributeSetter, m privilegesResourceModel, diags *diag.Diagnostics) {
	for name, field := range r.fields(&m) {
		diags.Append(to.SetAttribute(ctx, path.Root(name), field)...)
	}
}

func NewGroupPrivilegesResource() resource.Resource {
	return &privilegesResource{kind: groupPrivilegeKind}
}

func NewFolderPrivilegesResource() resource.Resource {
	return &privilegesResource{kind: folderPrivilegeKind}
}

func (r *privilegesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.target + "_privileges"
}

func (r *privilegesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	target := r.kind.target
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("Sets exactly which privileges one subject has on a %[1]s. The subject's other privileges on the %[1]s are revoked, and other subjects' privileges, including any Grouper grants by default, are left alone. Use one of these per %[1]s and subject.", target),
		Attributes: map[string]schema.Attribute{
			target: schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("UUID of the %[1]s, such as `grouper_%[1]s.example.id`.", target),
				Required:            true,
				PlanModifiers:       replace,
			},
			"subject": schema.StringAttribute{
				MarkdownDescription: "Subject ID of a person from the provider's `subject_source`. Exactly one of `subject` and `subject_group` is required.",
				Optional:            true,
				PlanModifiers:       replace,
			},
			"subject_group": schema.StringAttribute{
				MarkdownDescription: "UUID of a group, whose members all get the privileges. Exactly one of `subject` and `subject_group` is required.",
				Optional:            true,
				PlanModifiers:       replace,
			},
			"privileges": schema.SetAttribute{
				MarkdownDescription: fmt.Sprintf("Privileges the subject has on the %s, from `%s`.", target, strings.Join(r.kind.names, "`, `")),
				ElementType:         types.StringType,
				Required:            true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(r.kind.names...)),
				},
			},
		},
	}
}

func (r *privilegesResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("subject"), path.MatchRoot("subject_group")),
	}
}

func (r *privilegesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.provider = configuredProvider(req.ProviderData, &resp.Diagnostics)
}

func (r *privilegesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *privilegesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.apply(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}

func (r *privilegesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	state := r.get(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	privileges, err := r.kind.get(r.provider.client, ctx, state.Target.ValueString(), r.subject(state))

	// The target or subject group was deleted, taking the privileges with it
	if hasErrorCode(err, r.kind.notFoundCode, "SUBJECT_NOT_FOUND") {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read privileges", err.Error())
		return
	}

	state.Privileges = stringSet(privileges)
	r.put(ctx, &resp.State, state, &resp.Diagnostics)
}

func (r *privilegesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	state := r.get(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.kind.set(r.provider.client, ctx, state.Target.ValueString(), r.subject(state), nil)
	if err != nil && !hasErrorCode(err, r.kind.notFoundCode, "SUBJECT_NOT_FOUND") {
		resp.Diagnostics.AddError("Unable to revoke privileges", err.Error())
	}
}

// Imports from "<target name> <subject>", where the subject is a person's subject ID or a group's
// full name, which always contains a colon
func (r *privilegesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	targetName, subject, ok := strings.Cut(req.ID, " ")
	if !ok || targetName == "" || subject == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected \"<%s name> <subject ID or group name>\", got %q.", r.kind.target, req.ID))
		return
	}

	targetUUID, err := r.kind.find(r.provider.client, ctx, targetName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import privileges", err.Error())
		return
	}
	if targetUUID == "" {
		resp.Diagnostics.AddError(strings.ToUpper(r.kind.target[:1])+r.kind.target[1:]+" not found", fmt.Sprintf("No %s is currently named %q.", r.kind.target, targetName))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.kind.target), targetUUID)...)

	if !strings.Contains(subject, ":") {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subject"), subject)...)
		return
	}

	group, err := r.provider.client.FindGroup(ctx, subject)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import privileges", err.Error())
		return
	}
	if group == nil {
		resp.Diagnostics.AddError("Group not found", fmt.Sprintf("No group is currently named %q.", subject))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subject_group"), group.UUID)...)
}

// Sets the planned privileges and saves them as the new state
func (r *privilegesResource) apply(ctx context.Context, plan attributeGetter, state attributeSetter, diags *diag.Diagnostics) {
	m := r.get(ctx, plan, diags)
	if diags.HasError() {
		return
	}

	var privileges []string
	diags.Append(m.Privileges.ElementsAs(ctx, &privileges, false)...)
	if diags.HasError() {
		return
	}

	if err := r.kind.set(r.provider.client, ctx, m.Target.ValueString(), r.subject(m), privileges); err != nil {
		diags.AddError("Unable to set privileges", err.Error())
		return
	}

	r.put(ctx, state, m, diags)
}

func (r *privilegesResource) subject(m privilegesResourceModel) grouper.Subject {
	if !m.SubjectGroup.IsNull() {
		return grouper.Subject{ID: m.SubjectGroup.ValueString(), SourceID: grouper.GroupSource}
	}
	return grouper.Subject{ID: m.Subject.ValueString(), SourceID: r.provider.subjectSource}
}
