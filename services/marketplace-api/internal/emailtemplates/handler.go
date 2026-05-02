package emailtemplates

// handler.go — internal HTTP surface for the templates registry.
//
// Mounted under /internal alongside the rest of marketplace-api's
// internal endpoints. Cluster network policy + istio AuthorizationPolicy
// restrict who can hit /internal so we don't duplicate auth at the
// handler level (matches existing convention).
//
// Endpoints:
//
//   POST /internal/templates/refresh
//     Body: { "key": "..." } | {}
//     Evicts the template cache for the given key, or all keys when
//     the field is empty / absent. Called by tesserix-home immediately
//     after a cross-DB UPSERT into email_templates.
//
//   POST /internal/templates/:key/test
//     Body: { "to": "ops@mark8ly.com", "vars": { ... } }
//     Renders the template with the provided synthetic vars and sends
//     it via the configured TestSender. Used by the admin UI's "Send
//     test" button so an operator can validate a template before
//     flipping status from draft → published.
//
// The TestSender abstraction here is a tiny interface — different from
// the per-package mailers (orderdoc.Mailer, giftcard.Mailer) — because
// the test send doesn't need PDF attachments, theme loading, or any
// of the per-package shaping. It's just "render this key with these
// vars and POST to SendGrid." Each caller package can wire its own
// implementation; the simplest is a thin SendGrid v3 envelope.

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestSender is the minimal contract needed to deliver a test email.
// Implementations encode the rendered triple into whatever transport
// they prefer (typically SendGrid v3 HTTP API).
type TestSender interface {
	SendTest(ctx context.Context, to string, rendered Rendered) error
}

// Handler bundles the loader + test sender for the internal endpoints.
type Handler struct {
	Loader *Loader
	Sender TestSender
}

// NewHandler constructs a Handler.
func NewHandler(loader *Loader, sender TestSender) *Handler {
	return &Handler{Loader: loader, Sender: sender}
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
	_ = c.ShouldBindJSON(&body) // empty body is fine
	if body.Key == "" {
		h.Loader.InvalidateAll()
	} else {
		h.Loader.Invalidate(body.Key)
	}
	c.JSON(http.StatusOK, gin.H{"refreshed": true, "key": body.Key})
}

type testSendBody struct {
	To   string                 `json:"to"`
	Vars map[string]interface{} `json:"vars"`
}

func (h *Handler) testSend(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_key"})
		return
	}
	var body testSendBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if body.To == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_to"})
		return
	}

	// Bridge the JSON object → typed vars expected by templates.
	// We always render against a map so an operator can test any
	// template without needing a Go struct match — text/template +
	// html/template both accept maps.
	vars := body.Vars
	if vars == nil {
		vars = map[string]interface{}{}
	}

	rendered, err := h.Loader.Render(c.Request.Context(), key, vars)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "render_failed", "message": err.Error()})
		return
	}

	if h.Sender == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "no_sender",
			"message": "test sender not configured",
		})
		return
	}
	if err := h.Sender.SendTest(c.Request.Context(), body.To, rendered); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "send_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true, "key": key, "to": body.To})
}

// avoid unused-import lint when json is only used via struct tags
var _ = json.Marshal
