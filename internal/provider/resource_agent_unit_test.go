package provider

import (
	"testing"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildUpdateAgentRequestIncludesExistingAISAgentID(t *testing.T) {
	plan := AgentResourceModel{
		Name:               types.StringValue("Fleet Builder"),
		DisplayName:        types.StringValue("Fleet Builder"),
		Model:              types.StringValue("chatgpt:gpt-5.4"),
		ComputeClass:       types.StringValue("standard-2"),
		SystemPrompt:       types.StringValue("system"),
		AutopilotPrompt:    types.StringValue("autopilot"),
		Capabilities:       types.ListNull(types.StringType),
		Email:              types.StringValue("a9s-bot-6@mail.a9s.dev"),
		SubscriptionID:     types.StringNull(),
		ExistingAISAgentID: types.StringValue("eb7683e0-446e-449c-96a6-b5e3bd4f8195"),
		SessionType:        types.StringValue("worker"),
		TimerIntervalMs:    types.Int64Null(),
		TimerMessage:       types.StringNull(),
	}

	updateReq := client.UpdateAgentRequest{
		Name:               plan.Name.ValueString(),
		DisplayName:        plan.DisplayName.ValueString(),
		Model:              plan.Model.ValueString(),
		ComputeClass:       plan.ComputeClass.ValueString(),
		SystemPrompt:       plan.SystemPrompt.ValueString(),
		AutopilotPrompt:    plan.AutopilotPrompt.ValueString(),
		Email:              plan.Email.ValueString(),
		DoneForNowEnabled:  boolPtrFromTF(plan.DoneForNowEnabled),
		SubscriptionID:     plan.SubscriptionID.ValueString(),
		ExistingAISAgentID: plan.ExistingAISAgentID.ValueString(),
		SessionType:        plan.SessionType.ValueString(),
		TimerEnabled:       boolPtrFromTF(plan.TimerEnabled),
		TimerIntervalMs:    plan.TimerIntervalMs.ValueInt64(),
		TimerMessage:       plan.TimerMessage.ValueString(),
	}

	if updateReq.ExistingAISAgentID != "eb7683e0-446e-449c-96a6-b5e3bd4f8195" {
		t.Fatalf("expected existing AIS agent id to be forwarded, got %q", updateReq.ExistingAISAgentID)
	}
}
