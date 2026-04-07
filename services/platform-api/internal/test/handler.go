package test

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes e2e test helper endpoints. Only mounted when cfg.Env
// != "prod" (see cmd/server/main.go).
type Handler struct {
	recorder *TokenRecorder
}

// NewHandler constructs a Handler over the given recorder.
func NewHandler(recorder *TokenRecorder) *Handler {
	return &Handler{recorder: recorder}
}

// Register mounts test routes onto the given gin.RouterGroup. Mount
// against `/api/v1` so the routes live at `/api/v1/test/...`.
func (h *Handler) Register(r *gin.RouterGroup) {
	t := r.Group("/test")
	{
		t.GET("/verification/latest", h.latestToken)
	}
}

// latestToken returns the most-recent plaintext magic-link token for an
// email. Used by Playwright to bypass the inbox.
//
//	GET /api/v1/test/verification/latest?email=user@example.com
//	→ { "data": { "token": "..." } }
//	→ 404 { "error": "not_found", "message": "no token for email" }
func (h *Handler) latestToken(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "email query param is required",
		})
		return
	}
	tok, ok := h.recorder.Latest(email)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "no token for email",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"token": tok}})
}
