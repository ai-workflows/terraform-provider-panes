package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for the Panes API.
type Client struct {
	BaseURL    string
	Token      string
	OrgID      string
	HTTPClient *http.Client
}

// New creates a new Panes API client.
func New(baseURL, token, orgID string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		OrgID:   orgID,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Minute, // sandbox provisioning can be slow
		},
	}
}

// APIError represents an error response from the Panes API.
type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"error"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("panes api error (%d): %s", e.StatusCode, e.Message)
}

func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
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
		if err := json.Unmarshal(respBody, apiErr); err != nil {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshaling response: %w", err)
		}
	}

	return nil
}

// --- Agent types ---

type AgentSchedule struct {
	Shifts   []any  `json:"shifts"`
	OffShift string `json:"offShift"`
}

type AgentConfig struct {
	SessionType      string `json:"sessionType"`
	TimerEnabled     bool   `json:"timerEnabled"`
	TimerIntervalMs  int64  `json:"timerIntervalMs"`
	TimerMessage     string `json:"timerMessage"`
	DoneForNowEnabled bool  `json:"doneForNowEnabled"`
}

type Agent struct {
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	Email                 string         `json:"email"`
	DisplayName           string         `json:"displayName"`
	Model                 string         `json:"model"`
	Status                string         `json:"status"`
	TemplateID            string         `json:"templateId"`
	ComputeClass          string         `json:"computeClass"`
	SystemPrompt          string         `json:"systemPrompt"`
	AutopilotPrompt       string         `json:"autopilotPrompt"`
	Capabilities          []string       `json:"capabilities"`
	Schedule              *AgentSchedule `json:"schedule"`
	SubscriptionID        string         `json:"subscriptionId"`
	MachineID             string         `json:"machineId"`
	AISAgentID            string         `json:"aisAgentId"`
	OrchestratorSessionID string         `json:"orchestratorSessionId"`
	Config                *AgentConfig   `json:"config"`
	// Top-level fields kept for backward compat with create/update requests
	SessionType           string         `json:"sessionType"`
	TimerEnabled          bool           `json:"timerEnabled"`
	TimerIntervalMs       int64          `json:"timerIntervalMs"`
	TimerMessage          string         `json:"timerMessage"`
	CreatedAt             string         `json:"createdAt"`
	UpdatedAt             string         `json:"updatedAt"`
}

type CreateAgentRequest struct {
	Name               string         `json:"name"`
	DisplayName        string         `json:"displayName,omitempty"`
	TemplateID         string         `json:"templateId"`
	Model              string         `json:"model,omitempty"`
	ComputeClass       string         `json:"computeClass,omitempty"`
	SystemPrompt       string         `json:"systemPrompt,omitempty"`
	AutopilotPrompt    string         `json:"autopilotPrompt"`
	Capabilities       []string       `json:"capabilities,omitempty"`
	Email              string         `json:"email,omitempty"`
	DoneForNowEnabled  *bool          `json:"doneForNowEnabled,omitempty"`
	SubscriptionID     string         `json:"subscriptionId,omitempty"`
	ExistingAISAgentID string         `json:"existingAisAgentId,omitempty"`
	Schedule           *AgentSchedule `json:"schedule"`
	SessionType        string         `json:"sessionType,omitempty"`
	TimerEnabled       *bool          `json:"timerEnabled,omitempty"`
	TimerIntervalMs    int64          `json:"timerIntervalMs,omitempty"`
	TimerMessage       string         `json:"timerMessage,omitempty"`
}

type UpdateAgentRequest struct {
	Name               string         `json:"name,omitempty"`
	DisplayName        string         `json:"displayName,omitempty"`
	Model              string         `json:"model,omitempty"`
	ComputeClass       string         `json:"computeClass,omitempty"`
	SystemPrompt       string         `json:"systemPrompt,omitempty"`
	AutopilotPrompt    string         `json:"autopilotPrompt,omitempty"`
	Capabilities       []string       `json:"capabilities,omitempty"`
	Email              string         `json:"email,omitempty"`
	DoneForNowEnabled  *bool          `json:"doneForNowEnabled,omitempty"`
	SubscriptionID     string         `json:"subscriptionId,omitempty"`
	ExistingAISAgentID string         `json:"existingAisAgentId,omitempty"`
	Schedule           *AgentSchedule `json:"schedule,omitempty"`
	SessionType        string         `json:"sessionType,omitempty"`
	TimerEnabled       *bool          `json:"timerEnabled,omitempty"`
	TimerIntervalMs    int64          `json:"timerIntervalMs,omitempty"`
	TimerMessage       string         `json:"timerMessage,omitempty"`
}

// AgentIdentity holds the AIS identity details for an agent.
type AgentIdentity struct {
	AISAgentID  string              `json:"aisAgentId"`
	Identity    *AgentIdentityInfo  `json:"identity"`
	Credentials []AgentCredential   `json:"credentials"`
}

type AgentIdentityInfo struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type AgentCredential struct {
	Service string `json:"service"`
}

func (c *Client) GetAgentIdentity(ctx context.Context, id string) (*AgentIdentity, error) {
	var resp AgentIdentity
	if err := c.do(ctx, http.MethodGet, "/api/agents/"+id+"/identity", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Subscription types ---

type Subscription struct {
	ID                string `json:"id"`
	OrgID             string `json:"orgId"`
	UserID            string `json:"userId"`
	OrchestratorID    string `json:"orchestratorId"`
	Provider          string `json:"provider"`
	Label             string `json:"label"`
	Tier              string `json:"tier"`
	PlanTier          string `json:"planTier"` // legacy field from orchestrator
	Status            string `json:"status"`
	ProxyID           string `json:"proxyId"`
	RateLimitedAt     string `json:"rateLimitedAt"`
	RateLimitResetsAt string `json:"rateLimitResetsAt"`
	TokensToday       int    `json:"tokensToday"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type CreateSubscriptionRequest struct {
	Label    string `json:"label"`
	Provider string `json:"provider,omitempty"`
	Tier     string `json:"tier,omitempty"`
}

type UpdateSubscriptionRequest struct {
	Label string `json:"label,omitempty"`
	Tier  string `json:"tier,omitempty"`
}

func (c *Client) CreateSubscription(ctx context.Context, req CreateSubscriptionRequest) (*Subscription, error) {
	var resp Subscription
	if err := c.do(ctx, http.MethodPost, "/api/subscriptions", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) UpdateSubscription(ctx context.Context, id string, req UpdateSubscriptionRequest) (*Subscription, error) {
	var resp Subscription
	if err := c.do(ctx, http.MethodPatch, "/api/subscriptions/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DeleteSubscription(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/subscriptions/"+id, nil, nil)
}

type subscriptionListResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var resp subscriptionListResponse
	if err := c.do(ctx, http.MethodGet, "/api/subscriptions", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Subscriptions, nil
}

func (c *Client) GetSubscription(ctx context.Context, id string) (*Subscription, error) {
	// Try direct GET first (new Panes DB-backed endpoint)
	var resp Subscription
	if err := c.do(ctx, http.MethodGet, "/api/subscriptions/"+id, nil, &resp); err == nil {
		return &resp, nil
	}
	// Fallback to listing (for data source lookups by label)
	subs, err := c.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range subs {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, &APIError{StatusCode: 404, Message: "subscription not found"}
}

func (c *Client) CreateAgent(ctx context.Context, req CreateAgentRequest) (*Agent, error) {
	var resp Agent
	if err := c.do(ctx, http.MethodPost, "/api/agents", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetAgent(ctx context.Context, id string) (*Agent, error) {
	var resp Agent
	if err := c.do(ctx, http.MethodGet, "/api/agents/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) UpdateAgent(ctx context.Context, id string, req UpdateAgentRequest) (*Agent, error) {
	var resp Agent
	if err := c.do(ctx, http.MethodPatch, "/api/agents/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/agents/"+id, nil, nil)
}

func (c *Client) StartAgent(ctx context.Context, id string) (*Agent, error) {
	var resp Agent
	if err := c.do(ctx, http.MethodPost, "/api/agents/"+id+"/start", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) PauseAgent(ctx context.Context, id string) (*Agent, error) {
	var resp Agent
	if err := c.do(ctx, http.MethodPost, "/api/agents/"+id+"/pause", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Sandbox types ---

type Sandbox struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"`
	Image     string          `json:"image"`
	Compute   string          `json:"compute_class"`
	Owner     string          `json:"owner"`
	URL       string          `json:"url"`
	Cloud     string          `json:"cloud"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Metadata  SandboxMetadata `json:"metadata"`
}

type SandboxMetadata struct {
	Cloud             string `json:"cloud"`
	TailscaleHostname string `json:"tailscale_hostname"`
	GCPInstance       string `json:"gcp_instance"`
	GCPZone           string `json:"gcp_zone"`
	GCPProject        string `json:"gcp_project"`
	VMUrl             string `json:"vm_url"`
}

type CreateSandboxRequest struct {
	Image        string `json:"image,omitempty"`
	ComputeClass string `json:"compute_class,omitempty"`
	Cloud        string `json:"cloud,omitempty"`
	InstanceType string `json:"instance_type,omitempty"`
	NestedVirt   bool   `json:"nested_virt,omitempty"`
	DiskSize     int    `json:"disk_size,omitempty"`
	Zone         string `json:"zone,omitempty"`
	Project      string `json:"project,omitempty"`
}

func (c *Client) CreateSandbox(ctx context.Context, req CreateSandboxRequest) (*Sandbox, error) {
	var resp Sandbox
	if err := c.do(ctx, http.MethodPost, "/api/sandboxes", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetSandbox(ctx context.Context, id string) (*Sandbox, error) {
	return c.getSandbox(ctx, id, false)
}

func (c *Client) GetSandboxRefresh(ctx context.Context, id string) (*Sandbox, error) {
	return c.getSandbox(ctx, id, true)
}

func (c *Client) getSandbox(ctx context.Context, id string, refresh bool) (*Sandbox, error) {
	path := "/api/sandboxes/" + id
	if refresh {
		path += "?refresh=true"
	}
	var resp Sandbox
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DeleteSandbox(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/sandboxes/"+id, nil, nil)
}

func (c *Client) PauseSandbox(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/sandboxes/"+id+"/pause", nil, nil)
}

func (c *Client) ResumeSandbox(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/sandboxes/"+id+"/resume", nil, nil)
}

// WaitForSandbox polls until the sandbox reaches a terminal status.
func (c *Client) WaitForSandbox(ctx context.Context, id string, timeout time.Duration) (*Sandbox, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sb, err := c.GetSandbox(ctx, id)
		if err != nil {
			return nil, err
		}
		switch sb.Status {
		case "running":
			return sb, nil
		case "error", "destroyed":
			return nil, fmt.Errorf("sandbox reached terminal status: %s", sb.Status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return nil, fmt.Errorf("sandbox did not become ready within %s", timeout)
}
