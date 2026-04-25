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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ManagedAgentResource{}
	_ resource.ResourceWithImportState = &ManagedAgentResource{}
)

// ManagedAgentResource manages a single managed agent record in Orchestrator.
//
// This is the canonical home for "standing" agents — the internal staff
// agents in ai-workflows/agents (sre, fleet, monitoring, portal, etc.) that
// don't fit Fleet's engagement-shaped model. It's the non-deprecated successor
// to panes_agent for the standing-agent use case.
//
// Scope: this resource manages the Orchestrator agent record only (config,
// prompts, model, capabilities). It does not start/stop the agent — runtime
// lifecycle is operational, not IaC. Operators trigger start/stop via
// `POST /v1/managed-agents/:id/{start,stop}` against orchestrator directly.
type ManagedAgentResource struct {
	client *client.OrchestratorClient
}

type ManagedAgentResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	OrgID                 types.String `tfsdk:"org_id"`
	EngagementID          types.String `tfsdk:"engagement_id"`
	UserID                types.String `tfsdk:"user_id"`
	TemplateID            types.String `tfsdk:"template_id"`
	Name                  types.String `tfsdk:"name"`
	DisplayName           types.String `tfsdk:"display_name"`
	Role                  types.String `tfsdk:"role"`
	Model                 types.String `tfsdk:"model"`
	ComputeClass          types.String `tfsdk:"compute_class"`
	SystemPrompt          types.String `tfsdk:"system_prompt"`
	AutopilotPrompt       types.String `tfsdk:"autopilot_prompt"`
	Status                types.String `tfsdk:"status"`
	Intent                types.String `tfsdk:"intent"`
	SubscriptionID        types.String `tfsdk:"subscription_id"`
	AISAgentID            types.String `tfsdk:"ais_agent_id"`
	MachineID             types.String `tfsdk:"machine_id"`
	OrchestratorSessionID types.String `tfsdk:"orchestrator_session_id"`
}

func NewManagedAgentResource() resource.Resource {
	return &ManagedAgentResource{}
}

func (r *ManagedAgentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_managed_agent"
}

func (r *ManagedAgentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a single managed-agent record in Orchestrator. The non-deprecated successor to panes_agent for standing internal agents (sre/fleet/monitoring/etc.) that don't fit Fleet's engagement-shaped model. Manages config only — runtime lifecycle (start/stop) is operational and lives outside Terraform.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Managed agent ID. Computed unless explicitly imported.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Description: "Organization ID this agent belongs to. Defaults to the provider's org_id.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"engagement_id": schema.StringAttribute{
				Description: "Engagement ID, if this agent belongs to an engagement. Standing internal agents leave this null.",
				Optional:    true,
			},
			"user_id": schema.StringAttribute{
				Description: "Owning user ID.",
				Optional:    true,
			},
			"template_id": schema.StringAttribute{
				Description: "Agent template (developer-engineer, devops-sre, project-manager, custom).",
				Optional:    true,
			},
			"name": schema.StringAttribute{
				Description: "Agent name (identifier).",
				Required:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name.",
				Optional:    true,
			},
			"role": schema.StringAttribute{
				Description: "Agent role (e.g. builder, comms, sre).",
				Optional:    true,
			},
			"model": schema.StringAttribute{
				Description: "Model identifier (e.g. chatgpt:gpt-5.4, alias:default).",
				Optional:    true,
			},
			"compute_class": schema.StringAttribute{
				Description: "Sandbox compute class (e.g. standard, standard-2, performance).",
				Optional:    true,
			},
			"system_prompt": schema.StringAttribute{
				Description: "System prompt that defines the agent's role and identity.",
				Optional:    true,
			},
			"autopilot_prompt": schema.StringAttribute{
				Description: "Autopilot prompt that defines the agent's autonomous workflow.",
				Optional:    true,
			},
			"intent": schema.StringAttribute{
				Description: "Agent intent (e.g. run, stop). Defaults to stop.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				Description: "ChatGPT/proxy-router subscription ID to pin this agent to. Use data.panes_subscription to look up.",
				Optional:    true,
			},
			"ais_agent_id": schema.StringAttribute{
				Description: "AIS agent identity. Set on create to link to an existing AIS identity, or read on subsequent applies.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Current status (running, sleeping, paused, stopped, error). Read-only.",
				Computed:    true,
			},
			"machine_id": schema.StringAttribute{
				Description: "Sandbox/machine ID assigned to this agent. Read-only.",
				Computed:    true,
			},
			"orchestrator_session_id": schema.StringAttribute{
				Description: "Active Orchestrator session ID (set when the agent is running). Read-only.",
				Computed:    true,
			},
		},
	}
}

func (r *ManagedAgentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if pd.Orchestrator == nil {
		resp.Diagnostics.AddError(
			"Orchestrator client not configured",
			"Set orchestrator_url and either orchestrator_token (pre-minted JWT) or orchestrator_client_id+orchestrator_client_secret (OAuth2 client_credentials) on the provider block to manage panes_managed_agent resources.",
		)
		return
	}
	r.client = pd.Orchestrator
}

func (r *ManagedAgentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ManagedAgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateManagedAgentRequest{
		OrgID:           plan.OrgID.ValueString(),
		EngagementID:    plan.EngagementID.ValueString(),
		UserID:          plan.UserID.ValueString(),
		TemplateID:      plan.TemplateID.ValueString(),
		Name:            plan.Name.ValueString(),
		DisplayName:     plan.DisplayName.ValueString(),
		Role:            plan.Role.ValueString(),
		Model:           plan.Model.ValueString(),
		ComputeClass:    plan.ComputeClass.ValueString(),
		SystemPrompt:    plan.SystemPrompt.ValueString(),
		AutopilotPrompt: plan.AutopilotPrompt.ValueString(),
		Intent:          plan.Intent.ValueString(),
		SubscriptionID:  plan.SubscriptionID.ValueString(),
		AISAgentID:      plan.AISAgentID.ValueString(),
	}

	agent, err := r.client.CreateManagedAgent(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating managed agent", err.Error())
		return
	}

	r.mapAgentToState(agent, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ManagedAgentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ManagedAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent, err := r.client.GetManagedAgent(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading managed agent", err.Error())
		return
	}

	r.mapAgentToState(agent, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ManagedAgentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ManagedAgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ManagedAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.UpdateManagedAgentRequest{
		DisplayName:     stringPtr(plan.DisplayName),
		Role:            stringPtr(plan.Role),
		Model:           stringPtr(plan.Model),
		ComputeClass:    stringPtr(plan.ComputeClass),
		SystemPrompt:    stringPtr(plan.SystemPrompt),
		AutopilotPrompt: stringPtr(plan.AutopilotPrompt),
		Intent:          stringPtr(plan.Intent),
		SubscriptionID:  stringPtr(plan.SubscriptionID),
		AISAgentID:      stringPtr(plan.AISAgentID),
	}

	agent, err := r.client.UpdateManagedAgent(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating managed agent", err.Error())
		return
	}

	r.mapAgentToState(agent, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ManagedAgentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ManagedAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteManagedAgent(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return
		}
		resp.Diagnostics.AddError("Error deleting managed agent", err.Error())
	}
}

func (r *ManagedAgentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ManagedAgentResource) mapAgentToState(agent *client.ManagedAgent, state *ManagedAgentResourceModel) {
	state.ID = types.StringValue(agent.ID)
	state.Name = types.StringValue(agent.Name)
	state.OrgID = stringValueOrNull(agent.OrgID)
	state.EngagementID = stringValueOrNull(agent.EngagementID)
	state.UserID = stringValueOrNull(agent.UserID)
	state.TemplateID = stringValueOrNull(agent.TemplateID)
	state.DisplayName = stringValueOrNull(agent.DisplayName)
	state.Role = stringValueOrNull(agent.Role)
	state.Model = stringValueOrNull(agent.Model)
	state.ComputeClass = stringValueOrNull(agent.ComputeClass)
	state.SystemPrompt = stringValueOrNull(agent.SystemPrompt)
	state.AutopilotPrompt = stringValueOrNull(agent.AutopilotPrompt)
	state.Status = stringValueOrNull(agent.Status)
	state.Intent = stringValueOrNull(agent.Intent)
	state.SubscriptionID = stringValueOrNull(agent.SubscriptionID)
	state.AISAgentID = stringValueOrNull(agent.AISAgentID)
	state.MachineID = stringValueOrNull(agent.MachineID)
	state.OrchestratorSessionID = stringValueOrNull(agent.OrchestratorSessionID)
}

func stringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func stringValueOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
