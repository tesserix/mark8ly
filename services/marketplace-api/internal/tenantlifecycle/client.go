// Package tenantlifecycle is a marketplace-api client for platform-api's
// internal tenant suspend/unsuspend endpoints (#287).
//
// This is the platform-admin surface's first WRITE client. Every existing
// client (tenantdirectory, onboardingfunnel, estatecounts) is read-only.
// The status mapping below is the part that matters: an error must never
// be conflated with an empty or zero result. A caller that received
// {StoresAffected: 0, Changed: false} from a failed call would report
// "nothing to do" for a request that may well have suspended a tenant.
package tenantlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrUnavailable signals platform-api could not be reached, or answered 5xx.
var ErrUnavailable = errors.New("tenantlifecycle: platform-api unavailable")

// ErrNotFound signals platform-api returned 404 for a tenant id.
var ErrNotFound = errors.New("tenantlifecycle: tenant not found")

// ErrConflict signals platform-api returned 409 for an invalid status
// transition (e.g. suspending an archived tenant).
var ErrConflict = errors.New("tenantlifecycle: invalid status transition")

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// Result is the outcome of a suspend/unsuspend call.
type Result struct {
	TenantID       string `json:"tenant_id"`
	Status         string `json:"status"`
	StoresAffected int    `json:"stores_affected"`
	Changed        bool   `json:"changed"`
}

// Client calls platform-api's internal tenant lifecycle endpoints.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient constructs a Client. httpClient may be nil (defaults to a
// 5-second timeout). The secret is sent as X-Internal-Auth when non-empty.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: baseURL, secret: secret, http: httpClient}
}

// post issues a POST to path and decodes the response body into out.
//
// Status mapping: 200 decodes; 404 -> ErrNotFound; 409 -> ErrConflict;
// everything else, including every 5xx and any transport error,
// -> ErrUnavailable. A 200 whose body is truncated or unparseable is an
// error, never a zero result.
func (c *Client) post(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("tenantlifecycle: build request: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to decode below
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusConflict:
		return ErrConflict
	default:
		return fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("tenantlifecycle: decode: %w", err)
	}
	return nil
}

// Suspend calls POST /internal/tenants/:id/suspend.
func (c *Client) Suspend(ctx context.Context, tenantID string) (*Result, error) {
	return c.lifecycle(ctx, tenantID, "suspend")
}

// Unsuspend calls POST /internal/tenants/:id/unsuspend.
func (c *Client) Unsuspend(ctx context.Context, tenantID string) (*Result, error) {
	return c.lifecycle(ctx, tenantID, "unsuspend")
}

func (c *Client) lifecycle(ctx context.Context, tenantID, action string) (*Result, error) {
	var envelope struct {
		Data Result `json:"data"`
	}
	path := "/internal/tenants/" + url.PathEscape(tenantID) + "/" + action
	if err := c.post(ctx, path, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}
