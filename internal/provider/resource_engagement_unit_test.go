package provider

import (
	"context"
	"testing"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildEngagementConfig_WithAgentsAndRepos(t *testing.T) {
	repos, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"acme/web", "acme/api"})
	plan := EngagementResourceModel{
		Name: types.StringValue("Meridian"),
		Mode: types.StringValue("standard"),
		Agents: []EngagementAgentModel{
			{Role: types.StringValue("builder"), Count: types.Int64Value(2)},
			{Role: types.StringValue("qa"), Count: types.Int64Value(1)},
		},
		GithubRepos: repos,
	}

	got, err := buildEngagementConfig(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != "standard" {
		t.Fatalf("expected mode=standard, got %q", got.Mode)
	}
	if len(got.Agents) != 2 {
		t.Fatalf("expected 2 agent entries, got %d", len(got.Agents))
	}
	if got.Agents[0].Role != "builder" || got.Agents[0].Count != 2 {
		t.Fatalf("unexpected first agent: %+v", got.Agents[0])
	}
	if len(got.GithubRepos) != 2 || got.GithubRepos[1] != "acme/api" {
		t.Fatalf("unexpected github_repos: %v", got.GithubRepos)
	}
}

func TestBuildEngagementConfig_PerInstanceAttributes(t *testing.T) {
	plan := EngagementResourceModel{
		Name:        types.StringValue("Amboras"),
		Mode:        types.StringValue("standard"),
		GithubRepos: types.ListNull(types.StringType),
		Agents: []EngagementAgentModel{
			{
				Role:  types.StringValue("builder"),
				Count: types.Int64Value(2),
				Instances: []EngagementAgentInstanceModel{
					{
						Suffix: types.StringValue("1"),
						Focus:  types.StringValue("Meta Pixel events + Klaviyo"),
					},
					{
						Suffix:     types.StringValue("2"),
						Focus:      types.StringValue("Reviews system (Loox clone)"),
						AisAgentID: types.StringValue("ais-pre-existing-7"),
					},
				},
			},
		},
	}

	got, err := buildEngagementConfig(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("expected 1 agent role entry, got %d", len(got.Agents))
	}
	if len(got.Agents[0].Instances) != 2 {
		t.Fatalf("expected 2 instance entries, got %d", len(got.Agents[0].Instances))
	}
	if got.Agents[0].Instances[0].Focus != "Meta Pixel events + Klaviyo" {
		t.Fatalf("unexpected focus on instance 0: %q", got.Agents[0].Instances[0].Focus)
	}
	if got.Agents[0].Instances[1].AisAgentID != "ais-pre-existing-7" {
		t.Fatalf("unexpected ais_agent_id on instance 1: %q", got.Agents[0].Instances[1].AisAgentID)
	}
}

func TestBuildEngagementConfig_OmitsInstancesWhenAbsent(t *testing.T) {
	plan := EngagementResourceModel{
		Name:        types.StringValue("Vanilla"),
		Mode:        types.StringValue("standard"),
		GithubRepos: types.ListNull(types.StringType),
		Agents: []EngagementAgentModel{
			{Role: types.StringValue("builder"), Count: types.Int64Value(2)},
		},
	}

	got, err := buildEngagementConfig(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Agents[0].Instances != nil {
		t.Fatalf("expected Instances to be nil when not provided, got %+v", got.Agents[0].Instances)
	}
}

func TestBuildCreateEngagementRequest_ChatAgentMode(t *testing.T) {
	plan := EngagementResourceModel{
		Name:             types.StringValue("Support"),
		Mode:             types.StringValue("chat_agent"),
		SlackChannelName: types.StringValue("acme-support"),
		Agents:           nil,
		GithubRepos:      types.ListNull(types.StringType),
	}

	got, err := buildCreateEngagementRequest(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != "chat_agent" {
		t.Fatalf("expected mode=chat_agent, got %q", got.Mode)
	}
	if got.SlackChannelName != "acme-support" {
		t.Fatalf("expected slack_channel_name forwarded, got %q", got.SlackChannelName)
	}
	if len(got.Config.Agents) != 0 {
		t.Fatalf("expected zero agents for chat_agent, got %d", len(got.Config.Agents))
	}
}

// Modular role-prompts axes — see fleet/docs/design/modular-role-prompts.md.
// Plumbing-only test: confirms the resource model translates into the client
// wire shape Fleet expects (camelCase JSON tags on EngagementAgent*Config).
func TestBuildEngagementConfig_ModularPromptsAxes(t *testing.T) {
	paths, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"src/oidc/**"})
	plan := EngagementResourceModel{
		Name:              types.StringValue("Auth0Clone"),
		Mode:              types.StringValue("standard"),
		GithubRepos:       types.ListNull(types.StringType),
		MergePosture:      types.StringValue("auto_merge_on_ci_green"),
		OrgContext:        types.StringValue("All GCP deploys go through WIF to aiworkflows-prod."),
		EngagementContext: types.StringValue("Auth0-compatible OIDC service."),
		Agents: []EngagementAgentModel{
			{
				Role:                 types.StringValue("builder"),
				Count:                types.Int64Value(2),
				MergePosture:         types.StringValue("wait_for_agent_review"),
				BlockedBehavior:      types.StringValue("try_other_work"),
				PathsRequiringReview: paths,
				FreeformInstructions: types.StringValue("Prefer concise commit messages."),
				Instances: []EngagementAgentInstanceModel{
					{
						Suffix:               types.StringValue("1"),
						MergePosture:         types.StringValue("auto_merge_on_ci_green"),
						FreeformInstructions: types.StringValue("Focus on the OIDC kernel."),
					},
					{Suffix: types.StringValue("2")},
				},
			},
		},
		CommsConfig: []EngagementCommsConfigModel{
			{
				UpdateCadence:        types.StringValue("milestone"),
				EscalationThreshold:  types.StringValue("self_resolve_first"),
				FreeformInstructions: types.StringValue("Daily Slack summary."),
			},
		},
	}

	got, err := buildEngagementConfig(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MergePosture != "auto_merge_on_ci_green" {
		t.Fatalf("spec merge_posture: got %q", got.MergePosture)
	}
	if got.OrgContext == "" || got.EngagementContext == "" {
		t.Fatalf("expected context inputs forwarded")
	}
	if got.CommsConfig == nil {
		t.Fatal("expected comms_config to be non-nil")
	}
	if got.CommsConfig.UpdateCadence != "milestone" || got.CommsConfig.EscalationThreshold != "self_resolve_first" {
		t.Fatalf("comms_config axes mismatch: %+v", got.CommsConfig)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("expected 1 agent role, got %d", len(got.Agents))
	}
	a := got.Agents[0]
	if a.MergePosture != "wait_for_agent_review" {
		t.Fatalf("role merge_posture: got %q", a.MergePosture)
	}
	if a.BlockedBehavior != "try_other_work" {
		t.Fatalf("role blocked_behavior: got %q", a.BlockedBehavior)
	}
	if len(a.PathsRequiringReview) != 1 || a.PathsRequiringReview[0] != "src/oidc/**" {
		t.Fatalf("role paths_requiring_review: got %+v", a.PathsRequiringReview)
	}
	if a.FreeformInstructions != "Prefer concise commit messages." {
		t.Fatalf("role freeform_instructions: got %q", a.FreeformInstructions)
	}
	if len(a.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(a.Instances))
	}
	if a.Instances[0].MergePosture != "auto_merge_on_ci_green" {
		t.Fatalf("instance[0] merge_posture: got %q", a.Instances[0].MergePosture)
	}
	if a.Instances[0].FreeformInstructions != "Focus on the OIDC kernel." {
		t.Fatalf("instance[0] freeform_instructions: got %q", a.Instances[0].FreeformInstructions)
	}
	// Instance 1 set only suffix — all axes should be zero/empty.
	if a.Instances[1].MergePosture != "" {
		t.Fatalf("instance[1] should have empty merge_posture, got %q", a.Instances[1].MergePosture)
	}
}

func TestEngagementToModel_PreservesPlanInputsAndFillsComputed(t *testing.T) {
	plan := EngagementResourceModel{
		Name:             types.StringValue("Meridian"),
		SlackChannelName: types.StringValue("meridian"),
		GuidedPrompt:     types.StringValue("prompt"),
		Agents: []EngagementAgentModel{
			{Role: types.StringValue("builder"), Count: types.Int64Value(1)},
		},
		GithubRepos: types.ListNull(types.StringType),
	}

	eng := &client.Engagement{
		ID:               "eng-1",
		Name:             "Meridian",
		Status:           "active",
		Mode:             "standard",
		SlackChannelID:   "C1",
		CommsAgentID:     "comms-1",
		WorkerAgentIDs:   []string{"w1", "w2"},
		Config:           client.EngagementConfig{Mode: "standard"},
	}

	out := engagementToModel(eng, plan)
	if out.ID.ValueString() != "eng-1" {
		t.Fatalf("expected ID=eng-1, got %q", out.ID.ValueString())
	}
	if out.Status.ValueString() != "active" {
		t.Fatalf("expected status=active, got %q", out.Status.ValueString())
	}
	if out.Mode.ValueString() != "standard" {
		t.Fatalf("expected mode=standard, got %q", out.Mode.ValueString())
	}
	if out.SlackChannelID.ValueString() != "C1" {
		t.Fatalf("expected slack_channel_id=C1, got %q", out.SlackChannelID.ValueString())
	}
	if out.SlackChannelName.ValueString() != "meridian" {
		t.Fatalf("expected slack_channel_name preserved from plan")
	}
	if out.CommsAgentID.ValueString() != "comms-1" {
		t.Fatalf("expected comms_agent_id=comms-1, got %q", out.CommsAgentID.ValueString())
	}
	var workers []string
	diags := out.WorkerAgentIDs.ElementsAs(context.Background(), &workers, false)
	if diags.HasError() {
		t.Fatalf("worker list: %v", diags.Errors())
	}
	if len(workers) != 2 || workers[0] != "w1" || workers[1] != "w2" {
		t.Fatalf("unexpected workers: %v", workers)
	}
}
