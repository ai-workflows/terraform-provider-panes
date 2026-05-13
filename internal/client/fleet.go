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

// FleetClient is an HTTP client for the Fleet API.
//
// Used for engagement lifecycle resources. Auth is a portal-compatible JWT
// (sub + org_id claims) in the Authorization header; the Fleet-side
// portalAuthMiddleware validates it via the auth service's JWKS.
type FleetClient struct {
	BaseURL    string
	Token      string
	OrgID      string
	HTTPClient *http.Client
}

func NewFleet(baseURL, token, orgID string) *FleetClient {
	return &FleetClient{
		BaseURL: baseURL,
		Token:   token,
		OrgID:   orgID,
		HTTPClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *FleetClient) do(ctx context.Context, method, path string, body any, result any) error {
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
		// Fleet wraps responses in { data: ... }. Unwrap when present.
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(respBody, &wrapper); err == nil && len(wrapper.Data) > 0 {
			if err := json.Unmarshal(wrapper.Data, result); err != nil {
				return fmt.Errorf("unmarshaling response data: %w", err)
			}
			return nil
		}
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshaling response: %w", err)
		}
	}

	return nil
}

// --- Engagement (Team) types ---

// Modular role-prompts axes (fleet phases 2/3/4). Wire shape matches
// fleet's TS types — see fleet/src/types.ts and
// fleet/docs/design/modular-role-prompts.md. Pointer / omitempty fields
// keep "unset" distinct from "explicit empty" so the practitioner can
// declare-nothing without overwriting Fleet defaults.
type EngagementAgentInstanceConfig struct {
	Suffix     string `json:"suffix,omitempty"`
	Focus      string `json:"focus,omitempty"`
	AisAgentID string `json:"aisAgentId,omitempty"`
	// Per-instance paused flag. When true, Fleet's PUT
	// /api/teams/:id/config reconciler asks orchestrator to pause
	// the matching worker. Sent as `paused: true|false` in JSON; the
	// Fleet zod schema rejects other shapes.
	Paused *bool `json:"paused,omitempty"`
	// Phase 2 builder axes (per-instance overrides).
	MergePosture         string   `json:"mergePosture,omitempty"`
	BlockedBehavior      string   `json:"blockedBehavior,omitempty"`
	PathsRequiringReview []string `json:"pathsRequiringReview,omitempty"`
	// Phase 4 per-instance free-form additions (≤500 chars; truncated by Fleet at render).
	FreeformInstructions string `json:"freeformInstructions,omitempty"`
}

type EngagementAgentConfig struct {
	Role         string                          `json:"role"`
	Count        int                             `json:"count"`
	ComputeClass string                          `json:"computeClass,omitempty"`
	Model        string                          `json:"model,omitempty"`
	Instances    []EngagementAgentInstanceConfig `json:"instances,omitempty"`
	// Phase 2 builder axes (role-level defaults; per-instance overrides above).
	MergePosture         string   `json:"mergePosture,omitempty"`
	BlockedBehavior      string   `json:"blockedBehavior,omitempty"`
	PathsRequiringReview []string `json:"pathsRequiringReview,omitempty"`
	FreeformInstructions string   `json:"freeformInstructions,omitempty"`
}

// EngagementCommsConfig carries the comms agent's behavior axes. Comms
// is implicit (one per engagement, not in agents[]) so its axes live at
// the engagement level. See fleet/src/types.ts CommsConfig.
type EngagementCommsConfig struct {
	UpdateCadence        string `json:"updateCadence,omitempty"`
	EscalationThreshold  string `json:"escalationThreshold,omitempty"`
	FreeformInstructions string `json:"freeformInstructions,omitempty"`
}

type EngagementConfig struct {
	Mode         string                  `json:"mode,omitempty"`
	GuidedPrompt string                  `json:"guidedPrompt,omitempty"`
	Agents       []EngagementAgentConfig `json:"agents"`
	GithubRepos  []string                `json:"githubRepos,omitempty"`
	// Phase 5 engagement-wide builder merge_posture default. Per-role
	// agents[i].MergePosture overrides this; per-instance overrides that.
	MergePosture string `json:"mergePosture,omitempty"`
	// Phase 4 context inputs — Fleet appends to every agent's system prompt.
	OrgContext        string `json:"orgContext,omitempty"`
	EngagementContext string `json:"engagementContext,omitempty"`
	// Phase 3 comms agent axes.
	CommsConfig *EngagementCommsConfig `json:"commsConfig,omitempty"`
}

type DirectRepoAccess struct {
	Enabled   bool     `json:"enabled"`
	Usernames []string `json:"usernames"`
}

type Engagement struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	OrgID                string            `json:"orgId"`
	Status               string            `json:"status"`
	Mode                 string            `json:"mode,omitempty"`
	SlackChannelID       string            `json:"slackChannelId,omitempty"`
	SlackChannelName     string            `json:"slackChannelName,omitempty"`
	Repo                 string            `json:"repo,omitempty"`
	CommsAgentID         string            `json:"commsAgentId,omitempty"`
	WorkerAgentIDs       []string          `json:"workerAgentIds"`
	Config               EngagementConfig  `json:"config"`
	BillingRateCents     int64             `json:"billingRateCents"`
	CreatedAt            string            `json:"createdAt"`
	UpdatedAt            string            `json:"updatedAt"`
}

type CreateEngagementRequest struct {
	Name             string           `json:"name"`
	Mode             string           `json:"mode,omitempty"`
	SlackChannelName string           `json:"slackChannelName,omitempty"`
	Config           EngagementConfig `json:"config"`
}

type UpdateEngagementRequest struct {
	Name   string            `json:"name,omitempty"`
	Config *EngagementConfig `json:"config,omitempty"`
}

// Engagement CRUD is rooted at Portal's `/api/engagements/*` surface (which
// proxies to Fleet's `/api/teams/*` server-side). Engagement-repo CI and the
// TF provider — both external clients — talk to Portal so Fleet can be locked
// down to internal callers.
//
// Create still goes to /api/teams because Portal's /api/engagements collection
// is a 308 redirect to /api/teams, and POST + redirect is fragile in many
// HTTP clients. Read / Update / Delete go through the engagement-shaped paths
// directly.

func (c *FleetClient) CreateEngagement(ctx context.Context, req CreateEngagementRequest) (*Engagement, error) {
	var resp Engagement
	if err := c.do(ctx, http.MethodPost, "/api/teams", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *FleetClient) ListEngagements(ctx context.Context) ([]Engagement, error) {
	var resp []Engagement
	if err := c.do(ctx, http.MethodGet, "/api/teams", nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *FleetClient) GetEngagement(ctx context.Context, id string) (*Engagement, error) {
	var resp Engagement
	if err := c.do(ctx, http.MethodGet, "/api/engagements/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PutEngagementConfigRequest matches Portal's PUT /api/engagements/:id/config
// (which proxies to Fleet's PUT /api/teams/:id/config). Declarative shape —
// give Fleet the full desired state, Fleet diffs against current and calls
// Orchestrator / Slack / GitHub as needed.
type PutEngagementConfigRequest struct {
	SchemaVersion int                          `json:"schemaVersion,omitempty"`
	Engagement    *PutEngagementBlock          `json:"engagement,omitempty"`
	Agents        []EngagementAgentConfig      `json:"agents,omitempty"`
}

type PutEngagementBlock struct {
	Name             string                    `json:"name,omitempty"`
	Mode             string                    `json:"mode,omitempty"`
	GuidedPrompt     *string                   `json:"guidedPrompt,omitempty"`
	GithubRepos      *[]string                 `json:"githubRepos,omitempty"`
	ApprovalPolicy   string                    `json:"approvalPolicy,omitempty"`
	DirectRepoAccess *DirectRepoAccess         `json:"directRepoAccess,omitempty"`
}

// PutEngagementConfigResponse is the envelope Portal/Fleet returns from
// GET / PUT /api/engagements/:id/config.
type PutEngagementConfigResponse struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Engagement    EngagementConfigEnvelope  `json:"engagement"`
	Agents        []EngagementAgentConfig   `json:"agents"`
}

type EngagementConfigEnvelope struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	OrgID            string            `json:"orgId"`
	Status           string            `json:"status"`
	SlackChannelName string            `json:"slackChannelName"`
	SlackChannelURL  string            `json:"slackChannelUrl"`
	Repo             string            `json:"repo"`
	RepoURL          string            `json:"repoUrl"`
	Mode             string            `json:"mode"`
	GuidedPrompt     string            `json:"guidedPrompt"`
	GithubRepos      []string          `json:"githubRepos"`
	ApprovalPolicy   string            `json:"approvalPolicy"`
	DirectRepoAccess DirectRepoAccess  `json:"directRepoAccess"`
}

func (c *FleetClient) UpdateEngagementConfig(ctx context.Context, id string, req PutEngagementConfigRequest) (*PutEngagementConfigResponse, error) {
	var resp PutEngagementConfigResponse
	if err := c.do(ctx, http.MethodPut, "/api/engagements/"+id+"/config", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *FleetClient) DeleteEngagement(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/engagements/"+id, nil, nil)
}
