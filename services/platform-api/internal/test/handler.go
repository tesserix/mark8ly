package test

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes e2e test helper endpoints. Only mounted when cfg.Env
// != "prod" (see cmd/server/main.go).
type Handler struct {
	recorder      *TokenRecorder
	invitationRec *InvitationTokenRecorder
	sessionLookup func(ctx context.Context, email string) (any, error)
}

// NewHandler constructs a Handler over the given recorders. Both may
// be nil in contexts where that specific recorder isn't wired.
func NewHandler(recorder *TokenRecorder, invitationRec *InvitationTokenRecorder) *Handler {
	return &Handler{recorder: recorder, invitationRec: invitationRec}
}

// WithOnboardingSessionLookup wires the lookup behind
// GET /test/onboarding/sessions/latest (mark8ly#992). The function returns
// whatever the onboarding package wants serialised under "data"; nil
// leaves the route unmounted.
func (h *Handler) WithOnboardingSessionLookup(fn func(ctx context.Context, email string) (any, error)) *Handler {
	h.sessionLookup = fn
	return h
}

// Register mounts test routes onto the given gin.RouterGroup. Mount
// against `/api/v1` so the routes live at `/api/v1/test/...`.
func (h *Handler) Register(r *gin.RouterGroup) {
	t := r.Group("/test")
	{
		t.GET("/verification/latest", h.latestToken)
		t.GET("/invitations/latest", h.latestInvitation)
		if h.sessionLookup != nil {
			t.GET("/onboarding/sessions/latest", h.latestOnboardingSession)
		}
	}
}

// latestOnboardingSession returns the newest onboarding session for an
// email, so the e2e suite can assert what the server persisted — the
// sanitised acquisition record and the classification — rather than
// trusting what the browser sent.
//
//	GET /api/v1/test/onboarding/sessions/latest?email=user@example.com
//	→ { "data": { ...session... } }
func (h *Handler) latestOnboardingSession(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "email query param is required",
		})
		return
	}
	sess, err := h.sessionLookup(c.Request.Context(), email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sess})
}

// latestInvitation returns the most-recent plaintext invitation token
// for an email, across any tenant. The Phase P e2e suite uses this to
// bypass the invite email and drive the accept flow directly.
//
//	GET /api/v1/test/invitations/latest?email=user@example.com
//	→ { "data": { "token": "..." } }
func (h *Handler) latestInvitation(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "email query param is required",
		})
		return
	}
	if h.invitationRec == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "invitation recorder not wired",
		})
		return
	}
	tok, ok := h.invitationRec.LatestByEmail(email)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "no invitation token for email",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"token": tok}})
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
