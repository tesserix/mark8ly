package onboarding

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/verification"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for onboarding endpoints.
//
// Onboarding has its own verification subroutes that wrap the verification
// service with onboarding-specific behavior (mark the session as verified
// on success, return only the relevant fields).
type Handler struct {
	svc      *Service
	verifSvc *verification.Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, verifSvc *verification.Service) *Handler {
	return &Handler{svc: svc, verifSvc: verifSvc}
}

// Register mounts onboarding routes onto the given gin.RouterGroup.
func (h *Handler) Register(r *gin.RouterGroup) {
	o := r.Group("/onboarding")
	{
		o.POST("/sessions", h.createSession)
		o.GET("/sessions/:id", h.getSession)
		o.PATCH("/sessions/:id/draft", h.saveDraft)
		o.POST("/sessions/:id/verification/send", h.sendVerification)
		// Magic link verification: no session id in path because the magic
		// link only carries the token. The handler resolves the session
		// from the token's session_id.
		o.POST("/verify-token", h.verifyAndMarkSession)
		o.POST("/sessions/:id/complete", h.completeSession)
	}
}

func (h *Handler) createSession(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	sess, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": sess})
}

func (h *Handler) getSession(c *gin.Context) {
	sess, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sess})
}

func (h *Handler) saveDraft(c *gin.Context) {
	// Read raw body so callers can submit any JSON shape.
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	if err := h.svc.SaveDraft(c.Request.Context(), c.Param("id"), raw); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"saved": true}})
}

type sendVerificationRequest struct {
	BusinessName string `json:"business_name"`
}

func (h *Handler) sendVerification(c *gin.Context) {
	sessionID := c.Param("id")
	sess, err := h.svc.Get(c.Request.Context(), sessionID)
	if err != nil {
		respondError(c, err)
		return
	}
	var req sendVerificationRequest
	_ = c.ShouldBindJSON(&req) // body is optional

	if err := h.verifSvc.SendMagicLink(c.Request.Context(), sessionID, sess.Email, req.BusinessName); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"sent": true}})
}

type verifyTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// verifyAndMarkSession consumes a magic link token, marks the session
// verified, and returns the session ID + email so the caller (a server
// action in apps/onboarding) can immediately complete onboarding.
func (h *Handler) verifyAndMarkSession(c *gin.Context) {
	var req verifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	res, err := h.verifSvc.VerifyToken(c.Request.Context(), req.Token)
	if err != nil {
		respondError(c, err)
		return
	}
	if err := h.svc.MarkVerified(c.Request.Context(), res.SessionID); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"verified":   true,
		"session_id": res.SessionID,
		"email":      res.Email,
	}})
}

func (h *Handler) completeSession(c *gin.Context) {
	var req CompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	req.SessionID = c.Param("id")
	res, err := h.svc.Complete(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}

func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.JSON(ae.Status, gin.H{"error": ae.Code, "message": ae.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}
