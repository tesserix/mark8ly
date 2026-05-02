package notification

// handler.go — internal HTTP surface for the template registry.
//
// Mounted under /internal alongside tenant, store, invitation, auth.
// Cluster network policy restricts who can hit /internal so we don't
// duplicate auth at the handler level (matches the existing pattern).
//
// Endpoints:
//
//   POST /internal/templates/refresh
//     Body: { "key": "welcome" } | { "key": "" } | {}
//     Evicts the template cache for the given key, or all keys when
//     the field is empty / absent. Called by tesserix-home immediately
//     after a cross-DB UPSERT into email_templates so an authored
//     change is live within a request round-trip rather than waiting
//     for the 5-minute TTL.
//
//   POST /internal/templates/:key/test
//     Body: { "to": "ops@mark8ly.com", "vars": { ... } }
//     Renders the template with the provided synthetic vars and sends
//     it via the configured sender. Used by the admin UI's "Send test"
//     button so an operator can validate a template against their own
//     inbox before flipping status from draft → published.
//
// Both endpoints are POST (not GET) because they're not idempotent
// fetches — refresh mutates cache state, test triggers a side effect
// (an outbound email).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler bundles the loader and sender for the internal endpoints.
type Handler struct {
	Loader *Loader
	Sender Sender
	From   string
}

// NewHandler constructs a Handler. Loader and Sender must both be
// non-nil; the test-send endpoint needs a working sender to be useful.
func NewHandler(loader *Loader, sender Sender, from string) *Handler {
	return &Handler{Loader: loader, Sender: sender, From: from}
}

// Register mounts the routes under the given internal group.
func (h *Handler) Register(internal *gin.RouterGroup) {
	g := internal.Group("/templates")
	g.POST("/refresh", h.refresh)
	g.POST("/:key/test", h.testSend)
}

type refreshBody struct {
	Key string `json:"key"`
}

func (h *Handler) refresh(c *gin.Context) {
	var body refreshBody
	// An empty body is fine — fall through to "evict all".
	_ = c.ShouldBindJSON(&body)
	if body.Key == "" {
		h.Loader.InvalidateAll()
	} else {
		h.Loader.Invalidate(body.Key)
	}
	c.JSON(http.StatusOK, gin.H{"refreshed": true, "key": body.Key})
}

type testSendBody struct {
	To   string          `json:"to"`
	Vars json.RawMessage `json:"vars"`
}

func (h *Handler) testSend(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_key", "message": "template key is required"})
		return
	}
	var body testSendBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if body.To == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_to", "message": "to address is required"})
		return
	}

	// Decode the raw vars into the right typed struct based on key. The
	// embedded fallback path is type-safe so we have to materialize the
	// right Vars value here too. Drafts can render through the DB-only
	// path which doesn't care about Go types, but we want test-send to
	// work even with no DB row yet (operator wants to test an embedded
	// template via this same surface).
	vars, err := decodeTestSendVars(key, body.Vars)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_vars", "message": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30) // best-effort
	defer cancel()
	_ = ctx
	// Note: we use the request context directly rather than the timed
	// one above to avoid premature cancellation if SendGrid is slow.
	// The timeout is enforced by the SendGrid client itself (15s).
	msg, err := h.Loader.Render(c.Request.Context(), key, body.To, h.From, "", vars)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "render_failed", "message": err.Error()})
		return
	}
	if err := h.Sender.Send(c.Request.Context(), msg); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "send_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true, "key": key, "to": body.To})
}

// decodeTestSendVars maps a key onto its concrete Vars struct so the
// embedded fallback can render correctly. New keys must be added here
// to be testable from the admin UI.
func decodeTestSendVars(key string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	switch key {
	case "welcome":
		var v WelcomeVars
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "email_verification":
		var v EmailVerificationVars
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "invitation":
		var v InvitationVars
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "password_reset":
		var v PasswordResetVars
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("notification: no test-send vars binding for key %q", key)
}
