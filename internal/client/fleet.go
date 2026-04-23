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

type EngagementAgentConfig struct {
	Role         string `json:"role"`
	Count        int    `json:"count"`
	ComputeClass string `json:"computeClass,omitempty"`
	Model        string `json:"model,omitempty"`
}

type EngagementConfig struct {
	Mode         string                  `json:"mode,omitempty"`
	GuidedPrompt string                  `json:"guidedPrompt,omitempty"`
	Agents       []EngagementAgentConfig `json:"agents"`
	GithubRepos  []string                `json:"githubRepos,omitempty"`
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

func (c *FleetClient) CreateEngagement(ctx context.Context, req CreateEngagementRequest) (*Engagement, error) {
	var resp Engagement
	if err := c.do(ctx, http.MethodPost, "/api/teams", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *FleetClient) GetEngagement(ctx context.Context, id string) (*Engagement, error) {
	var resp Engagement
	if err := c.do(ctx, http.MethodGet, "/api/teams/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *FleetClient) UpdateEngagement(ctx context.Context, id string, req UpdateEngagementRequest) (*Engagement, error) {
	var resp Engagement
	if err := c.do(ctx, http.MethodPatch, "/api/teams/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *FleetClient) DeleteEngagement(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/teams/"+id, nil, nil)
}
