package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &AISAccountLinkResource{}
	_ resource.ResourceWithImportState = &AISAccountLinkResource{}
)

type AISAccountLinkResource struct {
	client *client.Client
}

type AISAccountLinkResourceModel struct {
	ID          types.String `tfsdk:"id"`
	AgentID     types.String `tfsdk:"agent_id"`
	AccountID   types.String `tfsdk:"account_id"`
	Permissions types.List   `tfsdk:"permissions"`
}

func NewAISAccountLinkResource() resource.Resource {
	return &AISAccountLinkResource{}
}

func (r *AISAccountLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ais_account_link"
}

// permissionValidator validates that each permission value is one of the allowed values.
type permissionValidator struct{}

func (v permissionValidator) Description(_ context.Context) string {
	return "each value must be one of: read, write, totp, browser_state"
}

func (v permissionValidator) MarkdownDescription(_ context.Context) string {
	return "each value must be one of: `read`, `write`, `totp`, `browser_state`"
}

func (v permissionValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var permissions []string
	diags := req.ConfigValue.ElementsAs(context.Background(), &permissions, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	allowed := map[string]bool{
		"read":          true,
		"write":         true,
		"totp":          true,
		"browser_state": true,
	}
	for _, perm := range permissions {
		if !allowed[perm] {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid permission",
				fmt.Sprintf("permission must be one of: read, write, totp, browser_state. Got: %s", perm),
			)
		}
	}
}

func (r *AISAccountLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Links an AIS account to an agent with specific permissions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Account link ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"agent_id": schema.StringAttribute{
				Description: "ID of the agent to link the account to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Description: "ID of the AIS account to link.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permissions": schema.ListAttribute{
				Description: "Permissions granted to the agent for this account (read, write, totp, browser_state).",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					permissionValidator{},
				},
			},
		},
	}
}

func (r *AISAccountLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if pd.Panes == nil {
		resp.Diagnostics.AddError("Panes client not configured", "Set PANES_TOKEN (or token = ...) to manage panes_ais_account_link resources.")
		return
	}
	r.client = pd.Panes
}

func (r *AISAccountLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AISAccountLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var permissions []string
	resp.Diagnostics.Append(plan.Permissions.ElementsAs(ctx, &permissions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateAISAccountLinkRequest{
		AgentID:     plan.AgentID.ValueString(),
		AccountID:   plan.AccountID.ValueString(),
		Permissions: permissions,
	}

	link, err := r.client.CreateAISAccountLink(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AIS account link", err.Error())
		return
	}

	r.mapToState(link, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AISAccountLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AISAccountLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	link, err := r.client.GetAISAccountLink(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AIS account link", err.Error())
		return
	}

	r.mapToState(link, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AISAccountLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// agent_id and account_id require replace; permissions changes require
	// delete + recreate since the API only supports POST/DELETE for links.
	// Terraform will handle this via ForceNew on the immutable fields and
	// a destroy-then-create for permission changes.

	var plan AISAccountLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AISAccountLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete old link
	err := r.client.DeleteAISAccountLink(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			// Already gone, continue to recreate
		} else {
			resp.Diagnostics.AddError("Error deleting old AIS account link", err.Error())
			return
		}
	}

	// Recreate with new permissions
	var permissions []string
	resp.Diagnostics.Append(plan.Permissions.ElementsAs(ctx, &permissions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateAISAccountLinkRequest{
		AgentID:     plan.AgentID.ValueString(),
		AccountID:   plan.AccountID.ValueString(),
		Permissions: permissions,
	}

	link, err := r.client.CreateAISAccountLink(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error recreating AIS account link", err.Error())
		return
	}

	r.mapToState(link, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AISAccountLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AISAccountLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteAISAccountLink(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return
		}
		resp.Diagnostics.AddError("Error deleting AIS account link", err.Error())
	}
}

func (r *AISAccountLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AISAccountLinkResource) mapToState(link *client.AISAccountLink, state *AISAccountLinkResourceModel) {
	state.ID = types.StringValue(link.ID)
	state.AgentID = types.StringValue(link.AgentID)
	state.AccountID = types.StringValue(link.AccountID)

	if len(link.Permissions) > 0 {
		state.Permissions, _ = types.ListValueFrom(context.Background(), types.StringType, link.Permissions)
	} else {
		state.Permissions, _ = types.ListValueFrom(context.Background(), types.StringType, []string{})
	}
}
