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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &AgentInstanceResource{}
	_ resource.ResourceWithImportState = &AgentInstanceResource{}
)

// AgentInstanceResource represents a running agent — the lifecycle binding
// between an agent config and a sandbox. Creating starts the agent,
// destroying stops it.
type AgentInstanceResource struct {
	client *client.Client
}

type AgentInstanceResourceModel struct {
	ID        types.String `tfsdk:"id"`
	AgentID   types.String `tfsdk:"agent_id"`
	Status    types.String `tfsdk:"status"`
	SessionID types.String `tfsdk:"session_id"`
	MachineID types.String `tfsdk:"machine_id"`
}

func NewAgentInstanceResource() resource.Resource {
	return &AgentInstanceResource{}
}

func (r *AgentInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent_instance"
}

func (r *AgentInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a running agent instance. Creating this resource starts the agent; destroying it stops the agent and cleans up its sandbox.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as agent_id (agent instances are 1:1 with agents).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"agent_id": schema.StringAttribute{
				Description: "ID of the panes_agent to start.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Agent status (running, sleeping, paused, stopped, error).",
				Computed:    true,
			},
			"session_id": schema.StringAttribute{
				Description: "Orchestrator session ID.",
				Computed:    true,
			},
			"machine_id": schema.StringAttribute{
				Description: "Sandbox/machine ID provisioned for this agent.",
				Computed:    true,
			},
		},
	}
}

func (r *AgentInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AgentInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AgentInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agentID := plan.AgentID.ValueString()
	tflog.Info(ctx, "Starting agent", map[string]any{"agent_id": agentID})

	agent, err := r.client.StartAgent(ctx, agentID)
	if err != nil {
		resp.Diagnostics.AddError("Error starting agent", err.Error())
		return
	}

	plan.ID = types.StringValue(agent.ID)
	plan.Status = types.StringValue(agent.Status)
	if agent.SessionID != "" {
		plan.SessionID = types.StringValue(agent.SessionID)
	} else {
		plan.SessionID = types.StringNull()
	}
	if agent.MachineID != "" {
		plan.MachineID = types.StringValue(agent.MachineID)
	} else {
		plan.MachineID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AgentInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AgentInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent, err := r.client.GetAgent(ctx, state.AgentID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}

	// If the agent is stopped, remove the instance from state
	if agent.Status == "stopped" {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Status = types.StringValue(agent.Status)
	if agent.SessionID != "" {
		state.SessionID = types.StringValue(agent.SessionID)
	} else {
		state.SessionID = types.StringNull()
	}
	if agent.MachineID != "" {
		state.MachineID = types.StringValue(agent.MachineID)
	} else {
		state.MachineID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AgentInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// agent_id requires replace, so no in-place updates
	resp.Diagnostics.AddError("Update not supported", "Agent instance changes require replacement.")
}

func (r *AgentInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AgentInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agentID := state.AgentID.ValueString()
	tflog.Info(ctx, "Stopping agent", map[string]any{"agent_id": agentID})

	// Pause the agent (stops autopilot, keeps session data)
	_, err := r.client.PauseAgent(ctx, agentID)
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return // already gone
		}
		resp.Diagnostics.AddError("Error stopping agent", err.Error())
	}
}

func (r *AgentInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by agent ID
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), req.ID)...)
}
