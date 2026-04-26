package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ProxyRouterClient is a thin HTTP client for proxy-router's account
// listing endpoint. Used by the panes_subscription data source so it can
// look up seat inventory without going through Panes's read-only façade.
//
// Auth: when the configured URL is a Cloud Run service URL (`*.run.app`),
// the client mints a Google-signed identity token from the GCE metadata
// server (audience = base URL) and sends it as Bearer. Cloud Run's
// `roles/run.invoker` granted to `allUsers` accepts any valid identity
// token. For non-Cloud-Run URLs (Tailscale internal, dev), no token is
// added — proxy-router enforces auth differently in those cases.
type ProxyRouterClient struct {
	BaseURL    string
	HTTPClient *http.Client

	tokenMu      sync.Mutex
	cachedToken  string
	cachedExpiry time.Time
}

func NewProxyRouter(baseURL string) *ProxyRouterClient {
	return &ProxyRouterClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *ProxyRouterClient) isCloudRun() bool {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(u.Hostname(), ".run.app")
}

// mintIdToken returns a Google-signed identity token scoped to BaseURL.
// Cached for ~55 minutes (Google ID tokens are 60min TTL). Returns
// empty string when the GCE metadata server is unreachable (i.e. not
// running on GCE).
func (c *ProxyRouterClient) mintIdToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" && time.Now().Add(5*time.Minute).Before(c.cachedExpiry) {
		return c.cachedToken, nil
	}

	audience := url.QueryEscape(c.BaseURL)
	mdURL := fmt.Sprintf(
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=%s",
		audience,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mdURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	cli := &http.Client{Timeout: 5 * time.Second}
	res, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata server unreachable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("metadata server returned %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	c.cachedToken = strings.TrimSpace(string(body))
	c.cachedExpiry = time.Now().Add(60 * time.Minute)
	return c.cachedToken, nil
}

// ProxyRouterAccount mirrors what proxy-router returns from
// /api/v1/accounts. Fields kept in lockstep with the Panes shim's
// SubscriptionView (panes/packages/control-plane/src/api/subscriptions.ts).
type ProxyRouterAccount struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Email     string `json:"email"`
	Provider  string `json:"provider"`
	Tier      string `json:"tier"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (c *ProxyRouterClient) ListAccounts(ctx context.Context) ([]ProxyRouterAccount, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/accounts", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.isCloudRun() {
		token, err := c.mintIdToken(ctx)
		if err != nil {
			// Fall through with no token; if proxy-router rejects, we get
			// a clear 4xx rather than a confusing metadata-server error.
		} else if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy-router request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("proxy-router %d: %s", resp.StatusCode, string(body))
	}

	var wrapped struct {
		Data []ProxyRouterAccount `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("decode proxy-router response: %w", err)
	}
	return wrapped.Data, nil
}
