package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.ResourceWithConfigure   = &folderResource{}
	_ resource.ResourceWithImportState = &folderResource{}
	_ resource.ResourceWithModifyPlan  = &folderResource{}
)

type folderResource struct {
	client *grouper.Client
}

type folderResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	DisplayExtension types.String `tfsdk:"display_extension"`
	Description      types.String `tfsdk:"description"`
}

func NewFolderResource() resource.Resource {
	return &folderResource{}
}

func (r *folderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (r *folderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Grouper folder. The parent folder must already exist.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID of the folder.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Full name of the folder, with segments separated by colons, such as `org:dept:apps`. Changing the last segment renames the folder, and changing its parent replaces it, since Grouper can't move folders.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(parentChanged, "Changing the parent folder replaces the folder.", "Changing the parent folder replaces the folder."),
				},
			},
			"display_extension": schema.StringAttribute{
				MarkdownDescription: "Display name of the folder itself, shown in the Grouper UI. Defaults to the last segment of `name`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the folder.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
		},
	}
}

// Whether a name change moves the folder or group to another parent folder
func parentChanged(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = parent(req.StateValue.ValueString()) != parent(req.PlanValue.ValueString())
}

func parent(name string) string {
	i := strings.LastIndex(name, ":")
	if i < 0 {
		return ""
	}
	return name[:i]
}

func lastSegment(name string) string {
	return name[strings.LastIndex(name, ":")+1:]
}

func (r *folderResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	defaultDisplayExtension(ctx, req, resp)
}

// Plans display_extension as the last segment of name when it isn't configured
func defaultDisplayExtension(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to plan when destroying
	if req.Plan.Raw.IsNull() {
		return
	}

	var configured, name types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("display_extension"), &configured)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if configured.IsNull() && !name.IsUnknown() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("display_extension"), lastSegment(name.ValueString()))...)
	}
}

func (r *folderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if p := configuredProvider(req.ProviderData, &resp.Diagnostics); p != nil {
		r.client = p.client
	}
}

func (r *folderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan folderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stem, err := r.client.CreateStem(ctx, plan.stem())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create folder", err.Error())
		return
	}

	plan.set(stem)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *folderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state folderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stem, err := r.client.FindStemByUUID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read folder", err.Error())
		return
	}

	// Deleted outside of OpenTofu, so plan to create it again
	if stem == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.set(stem)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *folderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state folderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stem, err := r.client.UpdateStem(ctx, state.ID.ValueString(), plan.stem())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update folder", err.Error())
		return
	}

	plan.set(stem)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *folderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state folderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteStem(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete folder", err.Error())
	}
}

// Imports by full name, since UUIDs aren't shown in the Grouper UI
func (r *folderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	stem, err := r.client.FindStem(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import folder", err.Error())
		return
	}
	if stem == nil {
		resp.Diagnostics.AddError("Folder not found", fmt.Sprintf("No folder is currently named %q.", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), stem.UUID)...)
}

func (m *folderResourceModel) stem() grouper.Stem {
	return grouper.Stem{
		Name:             m.Name.ValueString(),
		DisplayExtension: m.DisplayExtension.ValueString(),
		Description:      m.Description.ValueString(),
	}
}

func (m *folderResourceModel) set(stem *grouper.Stem) {
	m.ID = types.StringValue(stem.UUID)
	m.Name = types.StringValue(stem.Name)
	m.DisplayExtension = types.StringValue(stem.DisplayExtension)
	m.Description = types.StringValue(stem.Description)
}
