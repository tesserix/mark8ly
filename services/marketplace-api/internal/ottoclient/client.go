// Package ottoclient is a thin server-to-server HTTP client for the
// otto chat service. It exists so the marketplace-api can lazily pull a
// conversation's transcript when rendering the customer ticket detail
// page — the same ticket that slm-router escalated into the dashboard.
//
// Auth: every call carries the shared X-Internal-Auth header. The path
// itself is `/api/v1/internal/otto/...` which Otto restricts to S2S
// callers only; the customer's browser never reaches it directly.
package ottoclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls otto's internal-only HTTP surface.
type Client struct {
	baseURL string
	secret  string
	hc      *http.Client
}

// Option mutates a Client at construction time.
type Option func(*Client)

// WithHTTPClient overrides the default *http.Client (5s timeout). Useful
// for tests that need a custom RoundTripper.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.hc = hc }
}

// New returns a Client. baseURL is otto's reachable address, e.g.
// "http://otto.marketplace.svc.cluster.local:8089". secret is shared
// with otto's `OTTO_INTERNAL_AUTH` env var. Both values trimmed; a
// trailing slash on baseURL is stripped so callers can pass either form.
//
// Returns nil when baseURL OR secret is empty — the marketplace-api's
// transcript handler nil-checks the client and returns 404 to the
// customer when otto isn't wired (single-store dev without otto running).
func New(baseURL, secret string, opts ...Option) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	secret = strings.TrimSpace(secret)
	if baseURL == "" || secret == "" {
		return nil
	}
	c := &Client{
		baseURL: baseURL,
		secret:  secret,
		hc:      &http.Client{Timeout: 5 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// TranscriptMessage is the wire shape marketplace-api re-exports to the
// storefront. We deliberately project a narrow subset of otto's full
// message DTO — bodies + author hints — so changes to otto's internal
// schema can't leak into the customer-facing response.
type TranscriptMessage struct {
	ID         string `json:"id"`
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
}

// TranscriptConversation summarises the conversation's identifying
// fields. CaseID is otto's customer-visible identifier (e.g.
// "CS-260514-A7B2"), which is what the storefront UI displays next to
// the transcript.
type TranscriptConversation struct {
	ID        string `json:"id"`
	CaseID    string `json:"case_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

// Transcript is what GetTranscript returns. Empty Messages is valid (a
// new chat that escalated before the customer typed anything).
type Transcript struct {
	Conversation TranscriptConversation `json:"conversation"`
	Messages     []TranscriptMessage    `json:"messages"`
}

// ErrNotFound signals the conversation does not exist for the given
// tenant+store scope. The handler maps this to a 404 for the customer.
var ErrNotFound = errors.New("conversation not found")

// GetTranscript pulls a conversation's transcript scoped to the given
// tenant+store. The caller MUST have already verified the customer owns
// the ticket that links to this conversation — this client does no
// ownership check beyond the tenant/store scope on otto's side.
func (c *Client) GetTranscript(ctx context.Context, tenantID, storeID, conversationID string) (*Transcript, error) {
	if conversationID == "" {
		return nil, ErrNotFound
	}
	url := fmt.Sprintf("%s/api/v1/internal/otto/conversations/%s/transcript", c.baseURL, conversationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("otto transcript: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Auth", c.secret)
	req.Header.Set("X-Tenant-Id", tenantID)
	req.Header.Set("X-Store-Id", storeID)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("otto transcript: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return nil, fmt.Errorf("otto transcript: upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out Transcript
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("otto transcript: decode: %w", err)
	}
	return &out, nil
}
