package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// OrchestratorClient is an HTTP client for the Orchestrator REST API.
//
// Orchestrator authenticates via service JWTs minted from the Auth service
// over OAuth2 client_credentials. JWTs are short-lived (~5 min), so this
// client transparently mints + refreshes them as needed. Operators have
// two options for credentialing:
//
//  1. Pre-mint a JWT and pass it via Token / ORCHESTRATOR_TOKEN. Useful for
//     local development where you've already done `gcloud auth print-...`.
//     The client does not refresh in this mode — the token is used as-is
//     until it expires, then requests fail.
//
//  2. Provide ClientID + ClientSecret (or env: ORCHESTRATOR_CLIENT_ID /
//     ORCHESTRATOR_CLIENT_SECRET). The client mints + caches a JWT against
//     AuthURL (default: https://auth.infra.aiworkflows.com) and refreshes
//     30s before expiry. Standard for CI.
type OrchestratorClient struct {
	BaseURL      string
	OrgID        string
	HTTPClient   *http.Client
	StaticToken  string
	AuthURL      string
	ClientID     string
	ClientSecret string

	tokenMu      sync.Mutex
	cachedToken  string
	cachedExpiry time.Time
}

// NewOrchestrator creates a new orchestrator client with static-token auth.
// Suitable when the caller already has a service JWT (e.g. from gcloud or a
// CI step that minted one before terraform-apply).
func NewOrchestrator(baseURL, token, orgID string) *OrchestratorClient {
	return &OrchestratorClient{
		BaseURL:     baseURL,
		OrgID:       orgID,
		StaticToken: token,
		HTTPClient:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// NewOrchestratorWithCredentials creates a client that mints JWTs itself from
// the auth service via OAuth2 client_credentials. authURL may be empty to use
// the public default.
func NewOrchestratorWithCredentials(baseURL, authURL, clientID, clientSecret, orgID string) *OrchestratorClient {
	if authURL == "" {
		authURL = "https://auth.infra.aiworkflows.com"
	}
	return &OrchestratorClient{
		BaseURL:      baseURL,
		OrgID:        orgID,
		AuthURL:      authURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTPClient:   &http.Client{Timeout: 5 * time.Minute},
	}
}

// authToken returns a Bearer token for the next request. Static-token clients
// always return the static token; credential-based clients refresh on expiry.
func (c *OrchestratorClient) authToken(ctx context.Context) (string, error) {
	if c.StaticToken != "" {
		return c.StaticToken, nil
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return "", fmt.Errorf("no orchestrator credentials configured")
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" && time.Now().Add(30*time.Second).Before(c.cachedExpiry) {
		return c.cachedToken, nil
	}

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.AuthURL+"/auth/token", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("auth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("auth service token mint failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Data struct {
			Token     string `json:"token"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("auth token response: %w", err)
	}
	c.cachedToken = parsed.Data.Token
	c.cachedExpiry = time.Now().Add(time.Duration(parsed.Data.ExpiresIn) * time.Second)
	return c.cachedToken, nil
}

func (c *OrchestratorClient) do(ctx context.Context, method, path string, body, result any) error {
	token, err := c.authToken(ctx)
	if err != nil {
		return err
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if c.OrgID != "" {
		req.Header.Set("X-Org-Id", c.OrgID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		// Orchestrator wraps errors as { "error": { "code": "...", "message": "..." } }
		var wrapped struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &wrapped); err == nil && wrapped.Error.Message != "" {
			apiErr.Code = wrapped.Error.Code
			apiErr.Message = wrapped.Error.Message
		} else {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}

	if result != nil && len(respBody) > 0 {
		// Orchestrator wraps single-resource responses as { "data": {...} }.
		// Unwrap before unmarshaling into the caller's target.
		var wrapped struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(respBody, &wrapped); err == nil && len(wrapped.Data) > 0 {
			if err := json.Unmarshal(wrapped.Data, result); err != nil {
				return fmt.Errorf("unmarshaling data field: %w", err)
			}
			return nil
		}
		// Fallback: response wasn't wrapped, decode directly.
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshaling response: %w", err)
		}
	}
	return nil
}

// --- Managed agent types ---

// ManagedAgent mirrors the orchestrator's managed_agents row.
//
// Wire format note: orchestrator's REST API uses snake_case in REQUEST
// bodies (matching the Zod schemas in routes/managed-agents.ts) but
// camelCase in RESPONSE bodies (orchestrator's AgentStore returns
// internal camelCase representations directly via res.json). Hence each
// field carries two tags — `read:` is consulted on unmarshal, but Go's
// encoding/json doesn't support directional tags. Workaround: mark the
// JSON tag as the response form (camelCase) and use a separate
// CreateManagedAgentRequest / UpdateManagedAgentRequest struct with
// snake_case for outgoing payloads.
type ManagedAgent struct {
	ID                    string                 `json:"id"`
	OrgID                 string                 `json:"orgId,omitempty"`
	EngagementID          string                 `json:"engagementId,omitempty"`
	UserID                string                 `json:"userId,omitempty"`
	TemplateID            string                 `json:"templateId,omitempty"`
	Name                  string                 `json:"name"`
	DisplayName           string                 `json:"displayName,omitempty"`
	Role                  string                 `json:"role,omitempty"`
	Model                 string                 `json:"model,omitempty"`
	ComputeClass          string                 `json:"computeClass,omitempty"`
	SystemPrompt          string                 `json:"systemPrompt,omitempty"`
	AutopilotPrompt       string                 `json:"autopilotPrompt,omitempty"`
	Status                string                 `json:"status,omitempty"`
	Intent                string                 `json:"intent,omitempty"`
	SubscriptionID        string                 `json:"subscriptionId,omitempty"`
	MachineID             string                 `json:"machineId,omitempty"`
	OrchestratorSessionID string                 `json:"orchestratorSessionId,omitempty"`
	AISAgentID            string                 `json:"aisAgentId,omitempty"`
	Config                map[string]interface{} `json:"config,omitempty"`
	ErrorMessage          string                 `json:"errorMessage,omitempty"`
}

// CreateManagedAgentRequest matches the createAgentSchema in
// orchestrator/packages/service/src/routes/managed-agents.ts (snake_case).
type CreateManagedAgentRequest struct {
	ID              string                 `json:"id,omitempty"`
	OrgID           string                 `json:"org_id,omitempty"`
	EngagementID    string                 `json:"engagement_id,omitempty"`
	UserID          string                 `json:"user_id,omitempty"`
	TemplateID      string                 `json:"template_id,omitempty"`
	Name            string                 `json:"name"`
	DisplayName     string                 `json:"display_name,omitempty"`
	Role            string                 `json:"role,omitempty"`
	Model           string                 `json:"model,omitempty"`
	ComputeClass    string                 `json:"compute_class,omitempty"`
	SystemPrompt    string                 `json:"system_prompt,omitempty"`
	AutopilotPrompt string                 `json:"autopilot_prompt,omitempty"`
	Status          string                 `json:"status,omitempty"`
	Intent          string                 `json:"intent,omitempty"`
	SubscriptionID  string                 `json:"subscription_id,omitempty"`
	AISAgentID      string                 `json:"ais_agent_id,omitempty"`
	Config          map[string]interface{} `json:"config,omitempty"`
}

// UpdateManagedAgentRequest matches the updateAgentSchema. All fields are
// pointer types so omitempty distinguishes "don't change" from "clear".
// Strings use empty-string-as-clear since the API accepts null for nullable
// fields and "" for non-nullable (consult the schema).
type UpdateManagedAgentRequest struct {
	DisplayName     *string                `json:"display_name,omitempty"`
	Role            *string                `json:"role,omitempty"`
	Model           *string                `json:"model,omitempty"`
	ComputeClass    *string                `json:"compute_class,omitempty"`
	SystemPrompt    *string                `json:"system_prompt,omitempty"`
	AutopilotPrompt *string                `json:"autopilot_prompt,omitempty"`
	Status          *string                `json:"status,omitempty"`
	Intent          *string                `json:"intent,omitempty"`
	SubscriptionID  *string                `json:"subscription_id,omitempty"`
	AISAgentID      *string                `json:"ais_agent_id,omitempty"`
	Config          map[string]interface{} `json:"config,omitempty"`
}

func (c *OrchestratorClient) CreateManagedAgent(ctx context.Context, req CreateManagedAgentRequest) (*ManagedAgent, error) {
	var resp ManagedAgent
	if err := c.do(ctx, http.MethodPost, "/v1/managed-agents", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *OrchestratorClient) GetManagedAgent(ctx context.Context, id string) (*ManagedAgent, error) {
	var resp ManagedAgent
	if err := c.do(ctx, http.MethodGet, "/v1/managed-agents/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *OrchestratorClient) UpdateManagedAgent(ctx context.Context, id string, req UpdateManagedAgentRequest) (*ManagedAgent, error) {
	var resp ManagedAgent
	if err := c.do(ctx, http.MethodPatch, "/v1/managed-agents/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *OrchestratorClient) DeleteManagedAgent(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/managed-agents/"+id, nil, nil)
}

// StartManagedAgent kicks off a fresh orchestrator session for the agent.
// Returns the updated agent record (status will be "running").
func (c *OrchestratorClient) StartManagedAgent(ctx context.Context, id string) (*ManagedAgent, error) {
	var resp ManagedAgent
	if err := c.do(ctx, http.MethodPost, "/v1/managed-agents/"+id+"/start", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StopManagedAgent halts the active session. Subsequent calls are idempotent.
func (c *OrchestratorClient) StopManagedAgent(ctx context.Context, id string) (*ManagedAgent, error) {
	var resp ManagedAgent
	if err := c.do(ctx, http.MethodPost, "/v1/managed-agents/"+id+"/stop", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
