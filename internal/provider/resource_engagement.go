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

type EngagementAgentInstanceModel struct {
	Suffix     types.String `tfsdk:"suffix"`
	Focus      types.String `tfsdk:"focus"`
	AisAgentID types.String `tfsdk:"ais_agent_id"`
	// Per-instance paused flag. When true, Fleet asks orchestrator to
	// pause this worker so it stops accruing billable cycles.
	// Comms-agent flow: customer asks to halt → comms flips this in
	// terraform/main.tf → CI applies → Fleet calls
	// orchestrator.pauseAgent on the matching worker. CEV §4.
	Paused types.Bool `tfsdk:"paused"`
	// Modular role-prompts axes (per-instance overrides).
	// See fleet/docs/design/modular-role-prompts.md.
	MergePosture         types.String `tfsdk:"merge_posture"`
	BlockedBehavior      types.String `tfsdk:"blocked_behavior"`
	PathsRequiringReview types.List   `tfsdk:"paths_requiring_review"`
	FreeformInstructions types.String `tfsdk:"freeform_instructions"`
}

type EngagementAgentModel struct {
	Role         types.String                   `tfsdk:"role"`
	Count        types.Int64                    `tfsdk:"count"`
	ComputeClass types.String                   `tfsdk:"compute_class"`
	Model        types.String                   `tfsdk:"model"`
	Instances    []EngagementAgentInstanceModel `tfsdk:"instances"`
	// Role-level defaults; per-instance values above override.
	MergePosture         types.String `tfsdk:"merge_posture"`
	BlockedBehavior      types.String `tfsdk:"blocked_behavior"`
	PathsRequiringReview types.List   `tfsdk:"paths_requiring_review"`
	FreeformInstructions types.String `tfsdk:"freeform_instructions"`
}

// EngagementCommsConfigModel — comms agent behavior axes. Comms is
// implicit (one per engagement, not in agents[]) so its axes live at
// the engagement level.
type EngagementCommsConfigModel struct {
	UpdateCadence        types.String `tfsdk:"update_cadence"`
	EscalationThreshold  types.String `tfsdk:"escalation_threshold"`
	FreeformInstructions types.String `tfsdk:"freeform_instructions"`
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
	// Engagement-wide builder merge_posture default. Per-role and
	// per-instance values override.
	MergePosture types.String `tfsdk:"merge_posture"`
	// Context inputs Fleet appends to every agent's system prompt.
	OrgContext        types.String `tfsdk:"org_context"`
	EngagementContext types.String `tfsdk:"engagement_context"`
	// Comms-only behavior axes. List for terraform-plugin-framework
	// single-nested-object semantics — length 0 or 1.
	CommsConfig []EngagementCommsConfigModel `tfsdk:"comms_config"`
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
						"merge_posture": schema.StringAttribute{
							Description: "Role-level merge_posture default (builders only). Overrides the engagement-wide value; per-instance values override this. Values: auto_merge_on_ci_green | wait_for_agent_review | wait_for_customer_review | do_not_merge.",
							Optional:    true,
						},
						"blocked_behavior": schema.StringAttribute{
							Description: "How builders react when blocked. Values: try_other_work (default) | stop_and_pause.",
							Optional:    true,
						},
						"paths_requiring_review": schema.ListAttribute{
							Description: "Customer-repo path globs that force wait_for_agent_review semantics regardless of base merge_posture (e.g. ['src/oidc/**']).",
							ElementType: types.StringType,
							Optional:    true,
						},
						"freeform_instructions": schema.StringAttribute{
							Description: "Free-form addition appended to every agent of this role's system prompt. Truncated to 500 chars at render time.",
							Optional:    true,
						},
						"instances": schema.ListNestedAttribute{
							Description: "Per-instance attributes (suffix / focus / ais_agent_id). When set, length must equal count. Use when distinct workers in the same role need different focus areas (e.g. builder #1 'Meta Pixel', builder #2 'Reviews').",
							Optional:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"suffix": schema.StringAttribute{
										Description: "Stable suffix for this instance — used in the agent name (e.g. 'amboras-builder-1').",
										Optional:    true,
									},
									"focus": schema.StringAttribute{
										Description: "Free-form focus statement, substituted into the role's prompt template.",
										Optional:    true,
									},
									"ais_agent_id": schema.StringAttribute{
										Description: "Pre-existing AIS agent identity to bind. Empty = Fleet provisions one.",
										Optional:    true,
									},
									"paused": schema.BoolAttribute{
										Description: "Pause this worker. When flipped to true, Fleet calls orchestrator.pauseAgent on the matching agent so it stops accruing billable cycles. Flip back to false / omit to resume.",
										Optional:    true,
									},
									"merge_posture": schema.StringAttribute{
										Description: "Per-instance merge_posture override. Same values as the role-level field.",
										Optional:    true,
									},
									"blocked_behavior": schema.StringAttribute{
										Description: "Per-instance blocked_behavior override.",
										Optional:    true,
									},
									"paths_requiring_review": schema.ListAttribute{
										Description: "Per-instance paths_requiring_review override.",
										ElementType: types.StringType,
										Optional:    true,
									},
									"freeform_instructions": schema.StringAttribute{
										Description: "Per-instance free-form addition. Truncated to 500 chars at render time.",
										Optional:    true,
									},
								},
							},
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
			"merge_posture": schema.StringAttribute{
				Description: "Engagement-wide builder merge_posture default. Per-role and per-instance values override. Values: auto_merge_on_ci_green | wait_for_agent_review | wait_for_customer_review | do_not_merge.",
				Optional:    true,
			},
			"org_context": schema.StringAttribute{
				Description: "Org-level context appended to every agent's system prompt. Usually set once in portal /settings; engagement TF should leave this unset unless overriding.",
				Optional:    true,
			},
			"engagement_context": schema.StringAttribute{
				Description: "Per-engagement context appended to every agent's system prompt. Defaults to scope_doc on launch; set here to override.",
				Optional:    true,
			},
			"comms_config": schema.ListNestedAttribute{
				Description: "Comms agent behavior axes. At most one block — comms is implicit per engagement.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"update_cadence": schema.StringAttribute{
							Description: "How proactively comms posts Slack updates. Values: silent | milestone (default) | proactive.",
							Optional:    true,
						},
						"escalation_threshold": schema.StringAttribute{
							Description: "When comms surfaces builder-filed blockers to the customer. Values: escalate_early | self_resolve_first (default).",
							Optional:    true,
						},
						"freeform_instructions": schema.StringAttribute{
							Description: "Free-form supplementary instructions for the comms agent. Truncated to 500 chars at render time.",
							Optional:    true,
						},
					},
				},
			},
			"merge_posture": schema.StringAttribute{
				Description: "Engagement-wide builder merge_posture default. Per-role and per-instance values override. Values: auto_merge_on_ci_green | wait_for_agent_review | wait_for_customer_review | do_not_merge.",
				Optional:    true,
			},
			"org_context": schema.StringAttribute{
				Description: "Org-level context appended to every agent's system prompt. Usually set once in portal /settings; engagement TF should leave this unset unless overriding.",
				Optional:    true,
			},
			"engagement_context": schema.StringAttribute{
				Description: "Per-engagement context appended to every agent's system prompt. Defaults to scope_doc on launch; set here to override.",
				Optional:    true,
			},
			"comms_config": schema.ListNestedAttribute{
				Description: "Comms agent behavior axes. At most one block — comms is implicit per engagement.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"update_cadence": schema.StringAttribute{
							Description: "How proactively comms posts Slack updates. Values: silent | milestone (default) | proactive.",
							Optional:    true,
						},
						"escalation_threshold": schema.StringAttribute{
							Description: "When comms surfaces builder-filed blockers to the customer. Values: escalate_early | self_resolve_first (default).",
							Optional:    true,
						},
						"freeform_instructions": schema.StringAttribute{
							Description: "Free-form supplementary instructions for the comms agent. Truncated to 500 chars at render time.",
							Optional:    true,
						},
					},
				},
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

	configReq, err := buildPutEngagementConfigRequest(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid engagement config", err.Error())
		return
	}

	if _, err := r.fleet.UpdateEngagementConfig(ctx, state.ID.ValueString(), configReq); err != nil {
		resp.Diagnostics.AddError("Failed to update engagement", err.Error())
		return
	}

	// Re-read the engagement so state mirrors what Fleet committed (including
	// computed fields like the comms agent id and worker agent ids that the
	// declarative config endpoint doesn't surface).
	eng, err := r.fleet.GetEngagement(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to refresh engagement after update", err.Error())
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
		Mode:              plan.Mode.ValueString(),
		GuidedPrompt:      plan.GuidedPrompt.ValueString(),
		Agents:            []client.EngagementAgentConfig{},
		MergePosture:      plan.MergePosture.ValueString(),
		OrgContext:        plan.OrgContext.ValueString(),
		EngagementContext: plan.EngagementContext.ValueString(),
	}

	for _, a := range plan.Agents {
		agentConfig := client.EngagementAgentConfig{
			Role:                 a.Role.ValueString(),
			Count:                int(a.Count.ValueInt64()),
			ComputeClass:         a.ComputeClass.ValueString(),
			Model:                a.Model.ValueString(),
			MergePosture:         a.MergePosture.ValueString(),
			BlockedBehavior:      a.BlockedBehavior.ValueString(),
			FreeformInstructions: a.FreeformInstructions.ValueString(),
		}
		if !a.PathsRequiringReview.IsNull() && !a.PathsRequiringReview.IsUnknown() {
			var paths []string
			diags := a.PathsRequiringReview.ElementsAs(ctx, &paths, false)
			if diags.HasError() {
				return config, fmt.Errorf("invalid agents[%s].paths_requiring_review: %s", a.Role.ValueString(), diags.Errors()[0].Summary())
			}
			agentConfig.PathsRequiringReview = paths
		}
		// Per-instance attributes — only emit when the user actually provided
		// instances. Empty/nil instances tells Fleet "all instances of this
		// role are identical" (current behavior).
		if len(a.Instances) > 0 {
			instances := make([]client.EngagementAgentInstanceConfig, 0, len(a.Instances))
			for _, inst := range a.Instances {
				cfg := client.EngagementAgentInstanceConfig{
					Suffix:               inst.Suffix.ValueString(),
					Focus:                inst.Focus.ValueString(),
					AisAgentID:           inst.AisAgentID.ValueString(),
					MergePosture:         inst.MergePosture.ValueString(),
					BlockedBehavior:      inst.BlockedBehavior.ValueString(),
					FreeformInstructions: inst.FreeformInstructions.ValueString(),
				}
				if !inst.PathsRequiringReview.IsNull() && !inst.PathsRequiringReview.IsUnknown() {
					var paths []string
					diags := inst.PathsRequiringReview.ElementsAs(ctx, &paths, false)
					if diags.HasError() {
						return config, fmt.Errorf("invalid agents[%s].instances[].paths_requiring_review: %s", a.Role.ValueString(), diags.Errors()[0].Summary())
					}
					cfg.PathsRequiringReview = paths
				}
				// Only forward `paused` when the practitioner set it
				// explicitly. Null/unknown means "leave Fleet alone";
				// explicit true/false drives the pauseAgent/wakeAgent
				// reconciler on the Fleet side.
				if !inst.Paused.IsNull() && !inst.Paused.IsUnknown() {
					v := inst.Paused.ValueBool()
					cfg.Paused = &v
				}
				instances = append(instances, cfg)
			}
			agentConfig.Instances = instances
		}
		config.Agents = append(config.Agents, agentConfig)
	}

	if !plan.GithubRepos.IsNull() && !plan.GithubRepos.IsUnknown() {
		var repos []string
		diags := plan.GithubRepos.ElementsAs(ctx, &repos, false)
		if diags.HasError() {
			return config, fmt.Errorf("invalid github_repos: %s", diags.Errors()[0].Summary())
		}
		config.GithubRepos = repos
	}

	// Comms config is modeled as a List of length 0 or 1 (no native
	// single-nested attribute in this framework version, matching the
	// instances[] pattern above). Take the first row if present.
	if len(plan.CommsConfig) > 0 {
		c := plan.CommsConfig[0]
		config.CommsConfig = &client.EngagementCommsConfig{
			UpdateCadence:        c.UpdateCadence.ValueString(),
			EscalationThreshold:  c.EscalationThreshold.ValueString(),
			FreeformInstructions: c.FreeformInstructions.ValueString(),
		}
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

// buildPutEngagementConfigRequest shapes the resource model into the
// declarative envelope Portal's PUT /api/engagements/:id/config expects.
//
// Pointer fields are used where "omitted" and "explicit empty" need different
// semantics — `GuidedPrompt` and `GithubRepos` distinguish "leave the engagement
// alone" (nil) from "set this to empty" (non-nil pointer to zero value).
func buildPutEngagementConfigRequest(ctx context.Context, plan EngagementResourceModel) (client.PutEngagementConfigRequest, error) {
	config, err := buildEngagementConfig(ctx, plan)
	if err != nil {
		return client.PutEngagementConfigRequest{}, err
	}

	engagement := &client.PutEngagementBlock{
		Name: plan.Name.ValueString(),
		Mode: plan.Mode.ValueString(),
	}
	if !plan.GuidedPrompt.IsNull() && !plan.GuidedPrompt.IsUnknown() {
		gp := plan.GuidedPrompt.ValueString()
		engagement.GuidedPrompt = &gp
	}
	if !plan.GithubRepos.IsNull() && !plan.GithubRepos.IsUnknown() {
		repos := config.GithubRepos
		if repos == nil {
			repos = []string{}
		}
		engagement.GithubRepos = &repos
	}

	return client.PutEngagementConfigRequest{
		SchemaVersion: 1,
		Engagement:    engagement,
		Agents:        config.Agents,
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
		// Phase 2-5 modular-prompts axes — Fleet stores these in
		// EngagementConfig but we preserve from plan for now so the TF
		// diff loop converges. A future revision can read these back
		// from `eng.Config` to detect drift, once Fleet reliably
		// echoes them on the GET response.
		MergePosture:      plan.MergePosture,
		OrgContext:        plan.OrgContext,
		EngagementContext: plan.EngagementContext,
		CommsConfig:       plan.CommsConfig,
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
