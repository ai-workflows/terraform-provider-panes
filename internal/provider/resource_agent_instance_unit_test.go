package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type startAgentAlreadyRunningFixture struct {
	startCalls int
	getCalls   int
}

func TestAgentInstanceCreateAdoptsAlreadyRunningAgent(t *testing.T) {
	fixture := &startAgentAlreadyRunningFixture{}
	server := httptest.NewServer(http.HandlerFunc(fixture.handle))
	defer server.Close()

	ctx := context.Background()
	res := &AgentInstanceResource{client: client.New(server.URL, "test-token", "")}

	schemaResp := &resource.SchemaResponse{}
	res.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", schemaResp.Diagnostics)
	}

	plan, err := agentInstancePlanWithAgentID(ctx, schemaResp.Schema, "agent-123")
	if err != nil {
		t.Fatal(err)
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create returned diagnostics: %v", resp.Diagnostics)
	}
	if fixture.startCalls != 1 {
		t.Fatalf("expected one start call, got %d", fixture.startCalls)
	}
	if fixture.getCalls != 1 {
		t.Fatalf("expected one get call after already-running response, got %d", fixture.getCalls)
	}

	var got AgentInstanceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("reading response state returned diagnostics: %v", resp.Diagnostics)
	}
	if got.ID.ValueString() != "agent-123" {
		t.Fatalf("expected id agent-123, got %q", got.ID.ValueString())
	}
	if got.AgentID.ValueString() != "agent-123" {
		t.Fatalf("expected agent_id agent-123, got %q", got.AgentID.ValueString())
	}
	if got.Status.ValueString() != "running" {
		t.Fatalf("expected status running, got %q", got.Status.ValueString())
	}
	if got.SessionID.ValueString() != "session-456" {
		t.Fatalf("expected session id session-456, got %q", got.SessionID.ValueString())
	}
	if got.MachineID.ValueString() != "machine-789" {
		t.Fatalf("expected machine id machine-789, got %q", got.MachineID.ValueString())
	}
}

func (f *startAgentAlreadyRunningFixture) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/agents/agent-123/start":
		f.startCalls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Agent is already running"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/agents/agent-123":
		f.getCalls++
		_, _ = w.Write([]byte(`{"id":"agent-123","status":"running","orchestratorSessionId":"session-456","machineId":"machine-789"}`))
	default:
		http.Error(w, fmt.Sprintf("unexpected request %s %s", r.Method, r.URL.Path), http.StatusNotFound)
	}
}

func agentInstancePlanWithAgentID(ctx context.Context, s schema.Schema, agentID string) (tfsdk.Plan, error) {
	plan := tfsdk.Plan{Schema: s}
	diags := plan.Set(ctx, &AgentInstanceResourceModel{
		ID:        types.StringUnknown(),
		AgentID:   types.StringValue(agentID),
		Status:    types.StringUnknown(),
		SessionID: types.StringUnknown(),
		MachineID: types.StringUnknown(),
	})
	if diags.HasError() {
		return plan, fmt.Errorf("setting plan: %v", diags)
	}
	return plan, nil
}
