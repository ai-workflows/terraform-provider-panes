package provider

import (
	"context"
	"fmt"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &EngagementResource{}
	_ resource.ResourceWithImportState = &EngagementResource{}
)

// EngagementResource manages a Fleet engagement (called "team" in legacy API).
//
// Proxied through the Fleet API (/api/teams) rather than Panes directly —
// Fleet is the canonical owner of engagement lifecycle. See
// docs/architecture/design/platform-layers.md.
type EngagementResource struct {
	fleet *client.FleetClient
}

type EngagementAgentModel struct {
	Role         types.String `tfsdk:"role"`
	Count        types.Int64  `tfsdk:"count"`
	ComputeClass types.String `tfsdk:"compute_class"`
	Model        types.String `tfsdk:"model"`
}

type EngagementResourceModel struct {
	ID               types.String           `tfsdk:"id"`
	Name             types.String           `tfsdk:"name"`
	Mode             types.String           `tfsdk:"mode"`
	Status           types.String           `tfsdk:"status"`
	SlackChannelName types.String           `tfsdk:"slack_channel_name"`
	SlackChannelID   types.String           `tfsdk:"slack_channel_id"`
	GuidedPrompt     types.String           `tfsdk:"guided_prompt"`
	Agents           []EngagementAgentModel `tfsdk:"agents"`
	GithubRepos      types.List             `tfsdk:"github_repos"`
	CommsAgentID     types.String           `tfsdk:"comms_agent_id"`
	WorkerAgentIDs   types.List             `tfsdk:"worker_agent_ids"`
}

func NewEngagementResource() resource.Resource {
	return &EngagementResource{}
}

func (r *EngagementResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_engagement"
}

func (r *EngagementResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fleet engagement (team). Proxies to the Fleet API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Engagement ID assigned by Fleet.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Engagement name.",
				Required:    true,
			},
			"mode": schema.StringAttribute{
				Description: "Engagement mode: 'standard' (worker agents) or 'chat_agent' (comms-only).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Engagement status from Fleet.",
				Computed:    true,
			},
			"slack_channel_name": schema.StringAttribute{
				Description: "Slack channel name for the engagement. Optional.",
				Optional:    true,
			},
			"slack_channel_id": schema.StringAttribute{
				Description: "Provisioned Slack channel ID, if any.",
				Computed:    true,
			},
			"guided_prompt": schema.StringAttribute{
				Description: "Guided prompt passed to Fleet on creation.",
				Optional:    true,
			},
			"agents": schema.ListNestedAttribute{
				Description: "Worker agent role/count map. Empty list is chat_agent mode.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"role": schema.StringAttribute{
							Description: "Agent role: builder, specialist, qa, or pm.",
							Required:    true,
						},
						"count": schema.Int64Attribute{
							Description: "Number of agents of this role.",
							Required:    true,
						},
						"compute_class": schema.StringAttribute{
							Description: "Compute class: standard, standard-2, or performance.",
							Optional:    true,
						},
						"model": schema.StringAttribute{
							Description: "Override model for this role.",
							Optional:    true,
						},
					},
				},
			},
			"github_repos": schema.ListAttribute{
				Description: "GitHub repositories the engagement has access to.",
				ElementType: types.StringType,
				Optional:    true,
			},
			"comms_agent_id": schema.StringAttribute{
				Description: "Comms agent ID assigned by Fleet.",
				Computed:    true,
			},
			"worker_agent_ids": schema.ListAttribute{
				Description: "Worker agent IDs assigned by Fleet.",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
	}
}

func (r *EngagementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*ProviderClients)
	if !ok || pd.Fleet == nil {
		resp.Diagnostics.AddError(
			"Fleet client not configured",
			"The Fleet client is not configured on the provider. Set fleet_api_url and fleet_token (or FLEET_URL + FLEET_TOKEN env vars) on the provider block.",
		)
		return
	}

	r.fleet = pd.Fleet
}

func (r *EngagementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.fleet == nil {
		resp.Diagnostics.AddError("Fleet client not configured", "Configure fleet_api_url and fleet_token on the provider block.")
		return
	}

	var plan EngagementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq, err := buildCreateEngagementRequest(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid engagement config", err.Error())
		return
	}

	eng, err := r.fleet.CreateEngagement(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create engagement", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, engagementToModel(eng, plan))...)
}

func (r *EngagementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.fleet == nil {
		resp.Diagnostics.AddError("Fleet client not configured", "Configure fleet_api_url and fleet_token on the provider block.")
		return
	}

	var state EngagementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	eng, err := r.fleet.GetEngagement(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read engagement", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, engagementToModel(eng, state))...)
}

func (r *EngagementResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.fleet == nil {
		resp.Diagnostics.AddError("Fleet client not configured", "Configure fleet_api_url and fleet_token on the provider block.")
		return
	}

	var plan, state EngagementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := buildEngagementConfig(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid engagement config", err.Error())
		return
	}

	updateReq := client.UpdateEngagementRequest{
		Name:   plan.Name.ValueString(),
		Config: &config,
	}

	eng, err := r.fleet.UpdateEngagement(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update engagement", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, engagementToModel(eng, plan))...)
}

func (r *EngagementResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.fleet == nil {
		resp.Diagnostics.AddError("Fleet client not configured", "Configure fleet_api_url and fleet_token on the provider block.")
		return
	}

	var state EngagementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.fleet.DeleteEngagement(ctx, state.ID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.IsNotFound() {
			return
		}
		resp.Diagnostics.AddError("Failed to delete engagement", err.Error())
		return
	}
}

func (r *EngagementResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ============================================
// Helpers
// ============================================

func buildEngagementConfig(ctx context.Context, plan EngagementResourceModel) (client.EngagementConfig, error) {
	config := client.EngagementConfig{
		Mode:         plan.Mode.ValueString(),
		GuidedPrompt: plan.GuidedPrompt.ValueString(),
		Agents:       []client.EngagementAgentConfig{},
	}

	for _, a := range plan.Agents {
		config.Agents = append(config.Agents, client.EngagementAgentConfig{
			Role:         a.Role.ValueString(),
			Count:        int(a.Count.ValueInt64()),
			ComputeClass: a.ComputeClass.ValueString(),
			Model:        a.Model.ValueString(),
		})
	}

	if !plan.GithubRepos.IsNull() && !plan.GithubRepos.IsUnknown() {
		var repos []string
		diags := plan.GithubRepos.ElementsAs(ctx, &repos, false)
		if diags.HasError() {
			return config, fmt.Errorf("invalid github_repos: %s", diags.Errors()[0].Summary())
		}
		config.GithubRepos = repos
	}

	return config, nil
}

func buildCreateEngagementRequest(ctx context.Context, plan EngagementResourceModel) (client.CreateEngagementRequest, error) {
	config, err := buildEngagementConfig(ctx, plan)
	if err != nil {
		return client.CreateEngagementRequest{}, err
	}
	return client.CreateEngagementRequest{
		Name:             plan.Name.ValueString(),
		Mode:             plan.Mode.ValueString(),
		SlackChannelName: plan.SlackChannelName.ValueString(),
		Config:           config,
	}, nil
}

func engagementToModel(eng *client.Engagement, plan EngagementResourceModel) EngagementResourceModel {
	out := EngagementResourceModel{
		ID:               types.StringValue(eng.ID),
		Name:             types.StringValue(eng.Name),
		Status:           types.StringValue(eng.Status),
		Mode:             types.StringValue(resolveMode(eng)),
		SlackChannelName: plan.SlackChannelName,
		GuidedPrompt:     plan.GuidedPrompt,
		Agents:           plan.Agents,
		GithubRepos:      plan.GithubRepos,
	}

	if eng.SlackChannelID != "" {
		out.SlackChannelID = types.StringValue(eng.SlackChannelID)
	} else {
		out.SlackChannelID = types.StringNull()
	}

	if eng.CommsAgentID != "" {
		out.CommsAgentID = types.StringValue(eng.CommsAgentID)
	} else {
		out.CommsAgentID = types.StringNull()
	}

	workers := make([]attr.Value, len(eng.WorkerAgentIDs))
	for i, id := range eng.WorkerAgentIDs {
		workers[i] = types.StringValue(id)
	}
	workerList, _ := types.ListValue(types.StringType, workers)
	out.WorkerAgentIDs = workerList

	return out
}

func resolveMode(eng *client.Engagement) string {
	if eng.Mode != "" {
		return eng.Mode
	}
	if eng.Config.Mode != "" {
		return eng.Config.Mode
	}
	return "standard"
}
