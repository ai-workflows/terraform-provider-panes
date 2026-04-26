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

// AISClient is an HTTP client for the AIS REST API.
//
// AIS exposes admin routes at /api/v1/admin/accounts and
// /api/v1/admin/account-links. Earlier versions of this provider
// pointed those calls at the Panes URL on the assumption that Panes
// proxied admin/* through to AIS. That proxy was never wired up, so
// the original AIS resources 404'd on apply. This client gives the
// resources their own base URL + token so they call AIS directly.
//
// Auth is via a static admin token (Bearer). For staging/prod, the
// token is provisioned via auth-service or pre-baked in GSM
// (`ais-staging-admin-token` / `ais-prod-admin-token`).
type AISClient struct {
	BaseURL    string
	Token      string
	OrgID      string
	HTTPClient *http.Client
}

func NewAIS(baseURL, token, orgID string) *AISClient {
	return &AISClient{
		BaseURL: baseURL,
		Token:   token,
		OrgID:   orgID,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *AISClient) do(ctx context.Context, method, path string, body, result any) error {
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
		// AIS wraps errors as { "error": { "code": "...", "message": "..." } }.
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
		// AIS wraps single-resource responses as { "data": ... }. Unwrap.
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

// --- AIS Account ---

func (c *AISClient) CreateAISAccount(ctx context.Context, req CreateAISAccountRequest) (*AISAccount, error) {
	var resp AISAccount
	if err := c.do(ctx, http.MethodPost, "/api/v1/admin/accounts", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AISClient) GetAISAccount(ctx context.Context, id string) (*AISAccount, error) {
	var resp AISAccount
	if err := c.do(ctx, http.MethodGet, "/api/v1/admin/accounts/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AISClient) ListAISAccounts(ctx context.Context, orgID string) ([]AISAccount, error) {
	path := "/api/v1/admin/accounts"
	if orgID != "" {
		path += "?orgId=" + orgID
	}
	var resp []AISAccount
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *AISClient) UpdateAISAccount(ctx context.Context, id string, req UpdateAISAccountRequest) (*AISAccount, error) {
	var resp AISAccount
	if err := c.do(ctx, http.MethodPatch, "/api/v1/admin/accounts/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AISClient) DeleteAISAccount(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/admin/accounts/"+id, nil, nil)
}

// --- AIS Account Link ---

func (c *AISClient) CreateAISAccountLink(ctx context.Context, req CreateAISAccountLinkRequest) (*AISAccountLink, error) {
	var resp AISAccountLink
	if err := c.do(ctx, http.MethodPost, "/api/v1/admin/account-links", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AISClient) GetAISAccountLink(ctx context.Context, id string) (*AISAccountLink, error) {
	var resp AISAccountLink
	if err := c.do(ctx, http.MethodGet, "/api/v1/admin/account-links/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *AISClient) DeleteAISAccountLink(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/admin/account-links/"+id, nil, nil)
}
