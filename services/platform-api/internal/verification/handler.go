package verification

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for verification endpoints.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts verification routes onto the given gin.RouterGroup.
//
// All routes are public — verification is the gate to authentication, so
// it cannot itself require authentication.
func (h *Handler) Register(r *gin.RouterGroup) {
	v := r.Group("/verification")
	{
		v.POST("/send", h.sendCode)
		v.POST("/verify", h.verifyCode)
	}
}

type sendRequest struct {
	SessionID    string `json:"session_id" binding:"required"`
	Email        string `json:"email" binding:"required"`
	BusinessName string `json:"business_name"`
}

func (h *Handler) sendCode(c *gin.Context) {
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	if err := h.svc.SendCode(c.Request.Context(), req.SessionID, req.Email, req.BusinessName); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"sent": true}})
}

type verifyRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
}

func (h *Handler) verifyCode(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	if err := h.svc.Verify(c.Request.Context(), req.SessionID, req.Code); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"verified": true}})
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
