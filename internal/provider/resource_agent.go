package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	DisplayName     types.String `tfsdk:"display_name"`
	TemplateID      types.String `tfsdk:"template_id"`
	Model           types.String `tfsdk:"model"`
	Status          types.String `tfsdk:"status"`
	SystemPrompt    types.String `tfsdk:"system_prompt"`
	AutopilotPrompt types.String `tfsdk:"autopilot_prompt"`
	SubscriptionID  types.String `tfsdk:"subscription_id"`
	SessionID       types.String `tfsdk:"session_id"`
	MachineID       types.String `tfsdk:"machine_id"`
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
			"subscription_id": schema.StringAttribute{
				Description: "ChatGPT subscription ID to pin this agent to. Use data.panes_subscription to look up.",
				Optional:    true,
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

	createReq := client.CreateAgentRequest{
		Name:            plan.Name.ValueString(),
		DisplayName:     plan.DisplayName.ValueString(),
		TemplateID:      plan.TemplateID.ValueString(),
		Model:           plan.Model.ValueString(),
		SystemPrompt:    plan.SystemPrompt.ValueString(),
		AutopilotPrompt: plan.AutopilotPrompt.ValueString(),
		SubscriptionID:  plan.SubscriptionID.ValueString(),
		Schedule:        &client.AgentSchedule{Shifts: []any{}, OffShift: "sleep"},
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

	updateReq := client.UpdateAgentRequest{
		Name:            plan.Name.ValueString(),
		DisplayName:     plan.DisplayName.ValueString(),
		Model:           plan.Model.ValueString(),
		SystemPrompt:    plan.SystemPrompt.ValueString(),
		AutopilotPrompt: plan.AutopilotPrompt.ValueString(),
		SubscriptionID:  plan.SubscriptionID.ValueString(),
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

func (r *AgentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AgentResource) mapAgentToState(agent *client.Agent, state *AgentResourceModel) {
	state.ID = types.StringValue(agent.ID)
	state.Name = types.StringValue(agent.Name)
	state.Status = types.StringValue(agent.Status)

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
	} else {
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
