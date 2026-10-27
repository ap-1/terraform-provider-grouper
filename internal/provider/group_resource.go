package provider

import (
	"context"
	"fmt"

	"github.com/ap-1/terraform-provider-grouper/grouper"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure   = &groupResource{}
	_ resource.ResourceWithImportState = &groupResource{}
	_ resource.ResourceWithModifyPlan  = &groupResource{}
)

type groupResource struct {
	client *grouper.Client
}

type groupResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	DisplayExtension types.String `tfsdk:"display_extension"`
	Description      types.String `tfsdk:"description"`
}

func NewGroupResource() resource.Resource {
	return &groupResource{}
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Grouper group. Its folder must already exist.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the group.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Full name of the group, with segments separated by colons, such as `org:dept:apps:admins`. Changing the last segment renames the group, and changing its folder replaces it, since Grouper can't move groups.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(parentChanged, "Changing the folder replaces the group.", "Changing the folder replaces the group."),
				},
			},
			"display_extension": schema.StringAttribute{
				MarkdownDescription: "Display name of the group itself, shown in the Grouper UI. Defaults to the last segment of `name`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the group.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
		},
	}
}

func (r *groupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	defaultDisplayExtension(ctx, req, resp)
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if p := configuredProvider(req.ProviderData, &resp.Diagnostics); p != nil {
		r.client = p.client
	}
}

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.CreateGroup(ctx, plan.group())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create group", err.Error())
		return
	}

	plan.set(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.FindGroupByUUID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group", err.Error())
		return
	}

	// Deleted outside of OpenTofu, so plan to create it again
	if group == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.set(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.UpdateGroup(ctx, state.ID.ValueString(), plan.group())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update group", err.Error())
		return
	}

	plan.set(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteGroup(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete group", err.Error())
	}
}

// Imports by full name, since UUIDs aren't shown in the Grouper UI
func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	group, err := r.client.FindGroup(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import group", err.Error())
		return
	}
	if group == nil {
		resp.Diagnostics.AddError("Group not found", fmt.Sprintf("No group is currently named %q.", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), group.UUID)...)
}

func (m *groupResourceModel) group() grouper.Group {
	return grouper.Group{
		Name:             m.Name.ValueString(),
		DisplayExtension: m.DisplayExtension.ValueString(),
		Description:      m.Description.ValueString(),
	}
}

func (m *groupResourceModel) set(group *grouper.Group) {
	m.ID = types.StringValue(group.UUID)
	m.Name = types.StringValue(group.Name)
	m.DisplayExtension = types.StringValue(group.DisplayExtension)
	m.Description = types.StringValue(group.Description)
}
