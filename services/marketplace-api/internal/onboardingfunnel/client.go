// Package onboardingfunnel is a marketplace-api client for platform-api's
// internal onboarding-funnel endpoints (#283).
//
// Separate from internal/tenantdirectory on purpose: a funnel read is a
// different concern from a tenant directory read, and the two will diverge.
package onboardingfunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrUnavailable signals platform-api could not be reached, or answered 5xx.
// Callers MUST NOT treat this as an empty result: an empty funnel and an
// unreachable one are different answers, and conflating them shows a console
// operator "no activity" when the truth is "we could not ask".
var ErrUnavailable = errors.New("onboardingfunnel: platform-api unavailable")

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// FunnelCounts is one window's worth of funnel aggregates.
type FunnelCounts struct {
	Started       int64 `json:"started"`
	EmailVerified int64 `json:"email_verified"`
	Completed     int64 `json:"completed"`
	InFlight      int64 `json:"in_flight"`
	Abandoned     int64 `json:"abandoned"`
}

// FunnelWindow is the effective [from, to) window the stats were computed
// over, as returned by platform-api.
type FunnelWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// FunnelStats is the full funnel response.
type FunnelStats struct {
	FunnelCounts
	// MedianCompletionSeconds is nil when no session in the window
	// completed — upstream NULL must survive as Go nil, not silently
	// become 0 ("instant completion").
	MedianCompletionSeconds *float64     `json:"median_completion_seconds"`
	Last24h                 FunnelCounts `json:"last_24h"`
	Window                  FunnelWindow `json:"window"`
}

// Session is one row of the onboarding sessions list.
type Session struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Status          string     `json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	TenantID        *string    `json:"tenant_id,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
	CreatedAt       time.Time  `json:"created_at"`
	Abandoned       bool       `json:"abandoned"`
}

// SessionsResult is a page of sessions plus the unpaginated total.
type SessionsResult struct {
	Sessions []Session
	Total    int64
	Page     int
	Limit    int
}

// FunnelParams narrows a funnel query to a window. Zero values are omitted.
type FunnelParams struct {
	CreatedFrom time.Time
	CreatedTo   time.Time
}

// SessionsParams narrows a sessions query. Zero values are omitted.
type SessionsParams struct {
	CreatedFrom time.Time
	CreatedTo   time.Time
	Status      string
	Abandoned   *bool
	Page        int
	Limit       int
}

// Client calls platform-api's internal onboarding-funnel endpoints.
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
		return fmt.Errorf("onboardingfunnel: build request: %w", err)
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
		return fmt.Errorf("onboardingfunnel: platform-api %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("onboardingfunnel: decode: %w", err)
	}
	return nil
}

// windowValues encodes created_from/created_to as RFC3339 via url.Values, so
// the "+" in a non-UTC offset is percent-encoded rather than corrupted by a
// naive string concatenation.
func windowValues(from, to time.Time) url.Values {
	q := url.Values{}
	if !from.IsZero() {
		q.Set("created_from", from.Format(time.RFC3339))
	}
	if !to.IsZero() {
		q.Set("created_to", to.Format(time.RFC3339))
	}
	return q
}

// GetFunnel fetches the onboarding funnel stats for the given window.
func (c *Client) GetFunnel(ctx context.Context, p FunnelParams) (*FunnelStats, error) {
	q := windowValues(p.CreatedFrom, p.CreatedTo)

	path := "/internal/onboarding/funnel"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var envelope struct {
		Data FunnelStats `json:"data"`
	}
	if err := c.do(ctx, path, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}

// ListSessions fetches a page of onboarding sessions.
func (c *Client) ListSessions(ctx context.Context, p SessionsParams) (*SessionsResult, error) {
	q := windowValues(p.CreatedFrom, p.CreatedTo)
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Abandoned != nil {
		q.Set("abandoned", strconv.FormatBool(*p.Abandoned))
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	path := "/internal/onboarding/sessions"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var envelope struct {
		Data       []Session `json:"data"`
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	if err := c.do(ctx, path, &envelope); err != nil {
		return nil, err
	}

	// Allocate: upstream `"data": null` would otherwise become a nil slice
	// that the handler marshals as {} instead of [].
	sessions := envelope.Data
	if sessions == nil {
		sessions = []Session{}
	}

	return &SessionsResult{
		Sessions: sessions,
		Total:    envelope.Pagination.Total,
		Page:     envelope.Pagination.Page,
		Limit:    envelope.Pagination.Limit,
	}, nil
}
