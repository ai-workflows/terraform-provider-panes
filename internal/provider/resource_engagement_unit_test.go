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
