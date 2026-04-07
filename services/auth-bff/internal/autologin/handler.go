package autologin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler is the HTTP layer for autologin.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the autologin route onto the given gin.RouterGroup.
//
// The route is INTENTIONALLY public — the user comes in unauthenticated
// (they just finished onboarding, no session yet), presents an ID token,
// and we mint a session if everything checks out.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/auto-login", h.autoLogin)
}

type autoLoginRequest struct {
	IDToken          string `json:"id_token" binding:"required"`
	ExpectedTenantID string `json:"expected_tenant_id" binding:"required"`
	WorkspaceTenant  string `json:"workspace_tenant" binding:"required"`
}

func (h *Handler) autoLogin(c *gin.Context) {
	var req autoLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	res, err := h.svc.AutoLogin(c.Request.Context(), c.Writer, Request{
		IDToken:          req.IDToken,
		ExpectedTenantID: req.ExpectedTenantID,
		WorkspaceTenant:  req.WorkspaceTenant,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"uid":       res.UID,
			"email":     res.Email,
			"tenant_id": res.TenantID,
		},
	})
}

// respondError maps autologin errors to HTTP responses.
//
// 401: token invalid or tenant pool mismatch — the client is wrong
// 403: not a member of the tenant — the membership tuple is not (yet) visible;
//      after the retry budget this is a real "not authorized" answer
// 503: openfga is unreachable — the system is broken, retry the call
// 500: anything else (session mint failure, internal bugs)
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrTokenInvalid), errors.Is(err, ErrTenantMismatch):
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": err.Error(),
		})
	case errors.Is(err, ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "not_a_member",
			"message": "you are not a member of this tenant",
		})
	case errors.Is(err, ErrFGAUnreachable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "openfga_unreachable",
			"message": "authorization service is temporarily unavailable; please retry",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "an unexpected error occurred",
		})
	}
}
