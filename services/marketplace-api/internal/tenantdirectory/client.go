// Package tenantdirectory is a marketplace-api client for platform-api's
// internal tenant-directory endpoints (#277).
//
// Separate from internal/teamproxy on purpose: that package's job is team
// membership for a known tenant. A platform-wide directory read is a
// different concern with a different shape, and the two will diverge.
package tenantdirectory

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
// Callers MUST NOT treat this as an empty result: an empty directory and an
// unreachable one are different answers, and conflating them shows a console
// operator "no tenants" when the truth is "we could not ask".
var ErrUnavailable = errors.New("tenantdirectory: platform-api unavailable")

// ErrNotFound signals platform-api returned 404 for a tenant id.
var ErrNotFound = errors.New("tenantdirectory: tenant not found")

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// Tenant is the directory row.
type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	OwnerEmail string    `json:"owner_email"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// StoreSummary is one entry in a tenant's store rollup.
type StoreSummary struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TenantDetail is a tenant plus its store rollup.
type TenantDetail struct {
	Tenant
	Stores []StoreSummary `json:"stores"`
}

// ListResult is a page plus the unpaginated total.
type ListResult struct {
	Tenants []Tenant
	Total   int64
	Page    int
	Limit   int
}

// ListParams narrows a directory query. Zero values are omitted.
type ListParams struct {
	Q           string
	Status      string
	CreatedFrom time.Time
	CreatedTo   time.Time
	Page        int
	Limit       int
}

// Client calls platform-api's internal tenant-directory endpoints.
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
		return fmt.Errorf("tenantdirectory: build request: %w", err)
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
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("tenantdirectory: platform-api %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("tenantdirectory: decode: %w", err)
	}
	return nil
}

// List fetches a page of the tenant directory.
func (c *Client) List(ctx context.Context, p ListParams) (*ListResult, error) {
	q := url.Values{}
	if p.Q != "" {
		q.Set("q", p.Q)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if !p.CreatedFrom.IsZero() {
		q.Set("created_from", p.CreatedFrom.UTC().Format(time.RFC3339))
	}
	if !p.CreatedTo.IsZero() {
		q.Set("created_to", p.CreatedTo.UTC().Format(time.RFC3339))
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	path := "/internal/tenants"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var envelope struct {
		Data       []Tenant `json:"data"`
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
	tenants := envelope.Data
	if tenants == nil {
		tenants = []Tenant{}
	}

	return &ListResult{
		Tenants: tenants,
		Total:   envelope.Pagination.Total,
		Page:    envelope.Pagination.Page,
		Limit:   envelope.Pagination.Limit,
	}, nil
}

// Get fetches one tenant with its store rollup.
func (c *Client) Get(ctx context.Context, id string) (*TenantDetail, error) {
	var envelope struct {
		Data TenantDetail `json:"data"`
	}
	if err := c.do(ctx, "/internal/tenants/"+url.PathEscape(id)+"/detail", &envelope); err != nil {
		return nil, err
	}
	if envelope.Data.Stores == nil {
		envelope.Data.Stores = []StoreSummary{}
	}
	return &envelope.Data, nil
}
