// Package estateuserdir is a marketplace-api client for platform-api's
// internal estate staff directory (#278).
//
// Separate from internal/tenantdirectory for the same reason that package is
// separate from teamproxy: a platform-wide identity read is a different
// concern from a tenant read, and the two will diverge.
package estateuserdir

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
//
// Callers MUST NOT treat this as an empty result. An empty directory and an
// unreachable one are different answers, and a console operator shown "no
// users" would believe the first.
var ErrUnavailable = errors.New("estateuserdir: platform-api unavailable")

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// User is one estate staff identity, aggregated across tenants.
type User struct {
	Email       string `json:"email"`
	UserID      string `json:"user_id"`
	Roles       string `json:"roles"`
	TenantName  string `json:"tenant_name"`
	TenantCount int64  `json:"tenant_count"`
}

// ListParams narrows the directory.
type ListParams struct {
	Q     string
	Page  int
	Limit int
}

// ListResult is a page of users plus the unpaginated total.
type ListResult struct {
	Users []User
	Page  int
	Limit int
	Total int64
}

// Client calls platform-api's internal estate-user endpoint.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient constructs the client. A nil httpClient gets a 10s default.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: baseURL, secret: secret, http: httpClient}
}

// List fetches a page of estate staff.
func (c *Client) List(ctx context.Context, p ListParams) (*ListResult, error) {
	q := url.Values{}
	if p.Q != "" {
		q.Set("q", p.Q)
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	path := "/internal/users"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("estateuserdir: build request: %w", err)
	}
	req.Header.Set("X-Internal-Auth", c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("estateuserdir: unexpected status %d", resp.StatusCode)
	}

	var envelope struct {
		Data       []User `json:"data"`
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("estateuserdir: decode: %w", err)
	}
	// Allocate: a null `data` upstream must not become a nil slice here, or
	// the handler's own `?? []` guarantee depends on upstream's JSON.
	users := envelope.Data
	if users == nil {
		users = []User{}
	}
	return &ListResult{
		Users: users,
		Page:  envelope.Pagination.Page,
		Limit: envelope.Pagination.Limit,
		Total: envelope.Pagination.Total,
	}, nil
}
