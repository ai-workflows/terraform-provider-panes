package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &AgentResource{}
	_ resource.ResourceWithImportState = &AgentResource{}
)

type AgentResource struct {
	client *client.Client
}

type AgentResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	DisplayName        types.String `tfsdk:"display_name"`
	TemplateID         types.String `tfsdk:"template_id"`
	Model              types.String `tfsdk:"model"`
	ComputeClass       types.String `tfsdk:"compute_class"`
	Status             types.String `tfsdk:"status"`
	SystemPrompt       types.String `tfsdk:"system_prompt"`
	AutopilotPrompt    types.String `tfsdk:"autopilot_prompt"`
	Capabilities       types.List   `tfsdk:"capabilities"`
	SubscriptionID     types.String `tfsdk:"subscription_id"`
	Email              types.String `tfsdk:"email"`
	DoneForNowEnabled  types.Bool   `tfsdk:"done_for_now_enabled"`
	ExistingAISAgentID types.String `tfsdk:"existing_ais_agent_id"`
	AISAgentID         types.String `tfsdk:"ais_agent_id"`
	SessionID          types.String `tfsdk:"session_id"`
	MachineID          types.String `tfsdk:"machine_id"`
}

func NewAgentResource() resource.Resource {
	return &AgentResource{}
}

func (r *AgentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent"
}

func (r *AgentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Panes AI agent.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Agent ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Agent name (identifier).",
				Required:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name.",
				Optional:    true,
			},
			"template_id": schema.StringAttribute{
				Description: "Agent template (developer-engineer, devops-sre, project-manager, custom).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("custom"),
			},
			"model": schema.StringAttribute{
				Description: "Model identifier (e.g. chatgpt:gpt-5.4).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("chatgpt:gpt-5.4"),
			},
			"status": schema.StringAttribute{
				Description: "Agent status (running, sleeping, paused, stopped, error).",
				Computed:    true,
			},
			"system_prompt": schema.StringAttribute{
				Description: "System prompt that defines the agent's role and identity.",
				Optional:    true,
			},
			"autopilot_prompt": schema.StringAttribute{
				Description: "Autopilot prompt that defines the agent's autonomous workflow.",
				Required:    true,
			},
			"compute_class": schema.StringAttribute{
				Description: "Sandbox compute class (e.g. standard, standard-2, performance).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("standard-2"),
			},
			"capabilities": schema.ListAttribute{
				Description: "Agent capabilities (e.g. [\"browser\"] for web browsing).",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription_id": schema.StringAttribute{
				Description: "ChatGPT subscription ID to pin this agent to. Use data.panes_subscription to look up.",
				Optional:    true,
			},
			"email": schema.StringAttribute{
				Description: "Agent email address (must end with @mail.a9s.dev). Auto-generated if omitted.",
				Optional:    true,
				Computed:    true,
			},
			"done_for_now_enabled": schema.BoolAttribute{
				Description: "Whether the agent can call done_for_now to sleep after a work cycle. Defaults to false (continuous work).",
				Optional:    true,
			},
			"existing_ais_agent_id": schema.StringAttribute{
				Description: "Existing AIS agent ID to reuse identity/credentials from. If omitted, a new identity is created.",
				Optional:    true,
			},
			"ais_agent_id": schema.StringAttribute{
				Description: "AIS agent ID (auto-assigned on create, or set via existing_ais_agent_id).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"session_id": schema.StringAttribute{
				Description: "Active orchestrator session ID (set when agent is running).",
				Computed:    true,
			},
			"machine_id": schema.StringAttribute{
				Description: "Sandbox/machine ID assigned to this agent.",
				Computed:    true,
			},
		},
	}
}

func (r *AgentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *AgentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract capabilities from plan
	var capabilities []string
	if !plan.Capabilities.IsNull() && !plan.Capabilities.IsUnknown() {
		resp.Diagnostics.Append(plan.Capabilities.ElementsAs(ctx, &capabilities, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	createReq := client.CreateAgentRequest{
		Name:               plan.Name.ValueString(),
		DisplayName:        plan.DisplayName.ValueString(),
		TemplateID:         plan.TemplateID.ValueString(),
		Model:              plan.Model.ValueString(),
		ComputeClass:       plan.ComputeClass.ValueString(),
		SystemPrompt:       plan.SystemPrompt.ValueString(),
		AutopilotPrompt:    plan.AutopilotPrompt.ValueString(),
		Capabilities:       capabilities,
		Email:              plan.Email.ValueString(),
		DoneForNowEnabled:  boolPtrFromTF(plan.DoneForNowEnabled),
		SubscriptionID:     plan.SubscriptionID.ValueString(),
		ExistingAISAgentID: plan.ExistingAISAgentID.ValueString(),
		Schedule:           &client.AgentSchedule{Shifts: []any{}, OffShift: "sleep"},
	}

	agent, err := r.client.CreateAgent(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating agent", err.Error())
		return
	}

	r.mapAgentToState(agent, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AgentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent, err := r.client.GetAgent(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}

	r.mapAgentToState(agent, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AgentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updateCaps []string
	if !plan.Capabilities.IsNull() && !plan.Capabilities.IsUnknown() {
		resp.Diagnostics.Append(plan.Capabilities.ElementsAs(ctx, &updateCaps, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	updateReq := client.UpdateAgentRequest{
		Name:            plan.Name.ValueString(),
		DisplayName:     plan.DisplayName.ValueString(),
		Model:           plan.Model.ValueString(),
		ComputeClass:    plan.ComputeClass.ValueString(),
		SystemPrompt:    plan.SystemPrompt.ValueString(),
		AutopilotPrompt: plan.AutopilotPrompt.ValueString(),
		Capabilities:    updateCaps,
		Email:             plan.Email.ValueString(),
		DoneForNowEnabled: boolPtrFromTF(plan.DoneForNowEnabled),
		SubscriptionID:    plan.SubscriptionID.ValueString(),
	}

	agent, err := r.client.UpdateAgent(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating agent", err.Error())
		return
	}

	r.mapAgentToState(agent, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AgentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteAgent(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return
		}
		resp.Diagnostics.AddError("Error deleting agent", err.Error())
	}
}

func boolPtrFromTF(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	val := v.ValueBool()
	return &val
}

func (r *AgentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AgentResource) mapAgentToState(agent *client.Agent, state *AgentResourceModel) {
	state.ID = types.StringValue(agent.ID)
	state.Name = types.StringValue(agent.Name)
	state.Status = types.StringValue(agent.Status)

	if agent.Email != "" {
		state.Email = types.StringValue(agent.Email)
	} else if state.Email.IsUnknown() {
		state.Email = types.StringNull()
	}
	if agent.DisplayName != "" {
		state.DisplayName = types.StringValue(agent.DisplayName)
	} else {
		state.DisplayName = types.StringNull()
	}
	if agent.TemplateID != "" {
		state.TemplateID = types.StringValue(agent.TemplateID)
	}
	if agent.Model != "" {
		state.Model = types.StringValue(agent.Model)
	}
	if agent.SystemPrompt != "" {
		state.SystemPrompt = types.StringValue(agent.SystemPrompt)
	} else if state.SystemPrompt.IsNull() || state.SystemPrompt.IsUnknown() {
		// Only set null if state didn't already have a value (preserve plan value
		// when the API doesn't return systemPrompt in its response)
		state.SystemPrompt = types.StringNull()
	}
	if agent.AutopilotPrompt != "" {
		state.AutopilotPrompt = types.StringValue(agent.AutopilotPrompt)
	}
	if agent.SubscriptionID != "" {
		state.SubscriptionID = types.StringValue(agent.SubscriptionID)
	} else {
		state.SubscriptionID = types.StringNull()
	}
	if agent.ComputeClass != "" {
		state.ComputeClass = types.StringValue(agent.ComputeClass)
	}
	if len(agent.Capabilities) > 0 {
		caps := make([]types.String, len(agent.Capabilities))
		for i, c := range agent.Capabilities {
			caps[i] = types.StringValue(c)
		}
		state.Capabilities, _ = types.ListValueFrom(context.Background(), types.StringType, agent.Capabilities)
	} else if state.Capabilities.IsNull() || state.Capabilities.IsUnknown() {
		state.Capabilities, _ = types.ListValueFrom(context.Background(), types.StringType, []string{})
	}
	if agent.AISAgentID != "" {
		state.AISAgentID = types.StringValue(agent.AISAgentID)
	} else {
		state.AISAgentID = types.StringNull()
	}
	if agent.OrchestratorSessionID != "" {
		state.SessionID = types.StringValue(agent.OrchestratorSessionID)
	} else {
		state.SessionID = types.StringNull()
	}
	if agent.MachineID != "" {
		state.MachineID = types.StringValue(agent.MachineID)
	} else {
		state.MachineID = types.StringNull()
	}
}
