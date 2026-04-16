// Package audit is auth-bff's lightweight HTTP client for posting audit
// events to marketplace-api's /internal/audit-events ingest endpoint.
//
// All public methods are fire-and-forget: callers spin them off in a
// goroutine and never wait for the result. A failed audit POST must
// never break a login or logout flow — auth is on the critical path,
// audit logging is not.
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Client posts audit events to marketplace-api. baseURL empty disables
// the client (every method becomes a no-op) so dev environments without
// marketplace-api running still boot.
type Client struct {
	baseURL    string
	authSecret string
	http       *http.Client
	logger     *slog.Logger
}

// New constructs a Client. When baseURL is empty, every Emit call is a
// silent no-op — useful for local dev and tests.
func New(baseURL, authSecret string, logger *slog.Logger) *Client {
	return &Client{
		baseURL:    baseURL,
		authSecret: authSecret,
		http:       &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
	}
}

// Event mirrors the wire shape of marketplace-api's auditEventRequest.
// store_id is intentionally optional — marketplace-api falls back to the
// tenant's first active store when omitted, which is the common case for
// auth events that don't naturally carry a store scope.
type Event struct {
	TenantID     string         `json:"tenant_id"`
	StoreID      string         `json:"store_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Status       string         `json:"status,omitempty"`
	Severity     string         `json:"severity,omitempty"`
	ActorType    string         `json:"actor_type,omitempty"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	ActorEmail   string         `json:"actor_email,omitempty"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Emit posts a single event. Synchronous; spin off in a goroutine if the
// caller is on a request path. Returns no error — failures are logged.
func (c *Client) Emit(ctx context.Context, ev Event) {
	if c == nil || c.baseURL == "" {
		return
	}
	if ev.TenantID == "" || ev.Action == "" || ev.ResourceType == "" {
		c.logger.Warn("audit.Client.Emit: missing required fields",
			"tenant_id", ev.TenantID, "action", ev.Action, "resource_type", ev.ResourceType)
		return
	}

	body, err := json.Marshal(ev)
	if err != nil {
		c.logger.Warn("audit.Client.Emit: marshal failed", "err", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/audit-events", bytes.NewReader(body))
	if err != nil {
		c.logger.Warn("audit.Client.Emit: build request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authSecret != "" {
		req.Header.Set("X-Internal-Auth", c.authSecret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.Warn("audit.Client.Emit: post failed",
			"err", err, "action", ev.Action)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.logger.Warn("audit.Client.Emit: marketplace-api rejected event",
			"status", resp.StatusCode, "action", ev.Action)
	}
}

// EmitAsync wraps Emit in a fresh background context + goroutine so the
// calling request never waits. Use this from HTTP handlers.
func (c *Client) EmitAsync(ev Event) {
	if c == nil || c.baseURL == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.Emit(ctx, ev)
	}()
}
