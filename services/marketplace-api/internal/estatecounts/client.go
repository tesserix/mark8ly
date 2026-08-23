// Package estatecounts is a marketplace-api client for platform-api's
// internal platform-wide estate counts endpoint (#282).
package estatecounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrUnavailable signals platform-api could not be reached, or answered 5xx.
// Callers MUST NOT treat this as an empty result: an empty estate and an
// unreachable one are different answers, and conflating them shows a console
// operator "0 active tenants" when the truth is "we could not ask" — a KPI
// tile must never render an outage as if it were an emergency.
var ErrUnavailable = errors.New("estatecounts: platform-api unavailable")

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// Counts is the platform-wide estate rollup.
type Counts struct {
	TenantsActive int64 `json:"tenants_active"`
	StoresActive  int64 `json:"stores_active"`
}

// Client calls platform-api's internal estate-counts endpoint.
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

func (c *Client) do(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("estatecounts: build request: %w", err)
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
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("estatecounts: platform-api %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("estatecounts: decode: %w", err)
	}
	return nil
}

// Get fetches the platform-wide estate counts.
func (c *Client) Get(ctx context.Context) (*Counts, error) {
	var envelope struct {
		Data Counts `json:"data"`
	}
	if err := c.do(ctx, "/internal/estate/counts", &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}
