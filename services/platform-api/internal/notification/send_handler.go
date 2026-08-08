package notification

// send_handler.go — the general-purpose internal send endpoint.
//
//	POST /internal/notifications/send
//	Body: { "key": "login_otp", "to": "...", "tenant_id": "...", "vars": {...} }
//
// Distinct from /internal/templates/:key/test, which exists for an
// operator validating copy against their own inbox. This one is the
// production path other services call — currently auth-bff, for the
// sign-in code and the new-device alert.

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type sendBody struct {
	Key      string          `json:"key"`
	To       string          `json:"to"`
	TenantID string          `json:"tenant_id"`
	Vars     json.RawMessage `json:"vars"`
}

func (h *Handler) send(c *gin.Context) {
	var body sendBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if body.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_key", "message": "template key is required"})
		return
	}
	if body.To == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_to", "message": "to address is required"})
		return
	}

	// An unknown key fails here rather than rendering an empty email.
	vars, err := decodeVars(body.Key, body.Vars)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_vars", "message": err.Error()})
		return
	}

	msg, err := h.Loader.Render(c.Request.Context(), body.Key, body.To, h.From, body.TenantID, vars)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "render_failed", "message": err.Error()})
		return
	}
	if err := h.Sender.Send(c.Request.Context(), msg); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "send_failed", "message": err.Error()})
		return
	}

	// Deliberately no echo of key vars — a one-time code must not reach
	// a response body that an intermediary might log.
	c.JSON(http.StatusOK, gin.H{"sent": true, "key": body.Key})
}
