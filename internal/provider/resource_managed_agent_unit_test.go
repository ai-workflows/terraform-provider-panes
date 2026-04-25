package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ai-workflows/terraform-provider-panes/internal/client"
)

func TestOrchestratorClient_StaticTokenAuth(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("expected static bearer, got %q", got)
		}
		if got := r.Header.Get("X-Org-Id"); got != "org-1" {
			t.Fatalf("expected org header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {"id":"agent-1","name":"sre","status":"running"}}`))
	}))
	defer srv.Close()

	c := client.NewOrchestrator(srv.URL, "test-token", "org-1")
	got, err := c.GetManagedAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected the test server to be called")
	}
	if got.ID != "agent-1" || got.Name != "sre" || got.Status != "running" {
		t.Fatalf("unexpected agent: %+v", got)
	}
}

func TestOrchestratorClient_ClientCredentialsMintsAndCachesJWT(t *testing.T) {
	authCalls := 0
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls++
		if r.URL.Path != "/auth/token" {
			t.Fatalf("unexpected auth path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"token":"minted-jwt","expires_in":300}}`))
	}))
	defer authSrv.Close()

	orchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer minted-jwt" {
			t.Fatalf("expected minted bearer, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {"id":"a","name":"sre"}}`))
	}))
	defer orchSrv.Close()

	c := client.NewOrchestratorWithCredentials(orchSrv.URL, authSrv.URL, "cid", "csec", "")
	if _, err := c.GetManagedAgent(context.Background(), "a"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call must reuse the cached JWT — auth server should not be hit twice.
	if _, err := c.GetManagedAgent(context.Background(), "a"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if authCalls != 1 {
		t.Fatalf("expected auth service to be called once (cached), got %d", authCalls)
	}
}

func TestOrchestratorClient_NotFoundIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"not found"}}`))
	}))
	defer srv.Close()

	c := client.NewOrchestrator(srv.URL, "t", "")
	_, err := c.GetManagedAgent(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if !apiErr.IsNotFound() {
		t.Fatalf("expected IsNotFound to be true, got status %d", apiErr.StatusCode)
	}
}
