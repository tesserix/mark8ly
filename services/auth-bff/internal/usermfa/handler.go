package usermfa

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler is the HTTP layer for TOTP enrollment. Routes mount under
// /api/v1/mfa and are called by marketplace-api on behalf of the admin
// Account Settings page — user identity arrives on X-User-Id.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler constructs a Handler. svc may be nil — in that case every
// endpoint returns 501 Not Implemented.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the MFA routes onto the given gin.RouterGroup.
//
//	GET    /mfa/status    — returns { enabled: bool }
//	POST   /mfa/enable    — generates secret, returns QR data URL + otpauth URI
//	POST   /mfa/verify    — accepts 6-digit code, flips enabled=true
//	POST   /mfa/disable   — clears enrollment
func (h *Handler) Register(r *gin.RouterGroup) {
	r.GET("/mfa/status", h.status)
	r.POST("/mfa/enable", h.enable)
	r.POST("/mfa/verify", h.verify)
	r.POST("/mfa/disable", h.disable)
}

// userFromHeaders pulls X-User-Id (forwarded by marketplace-api) and
// the email, if present. auth-bff owns the session cookie but upstream
// callers only get these headers.
func userFromHeaders(c *gin.Context) (string, string, bool) {
	userID := c.GetHeader("X-User-Id")
	email := c.GetHeader("X-User-Email")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_session",
			"message": "no user context on request",
		})
		return "", "", false
	}
	return userID, email, true
}

func (h *Handler) status(c *gin.Context) {
	userID, _, ok := userFromHeaders(c)
	if !ok {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": false}})
		return
	}
	st, err := h.svc.GetStatus(c.Request.Context(), userID)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("mfa: status", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to read MFA status",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": st.Enabled}})
}

func (h *Handler) enable(c *gin.Context) {
	userID, email, ok := userFromHeaders(c)
	if !ok {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "MFA service not configured",
		})
		return
	}
	// Authenticator apps show "Issuer: AccountName", so use the email
	// when available, else fall back to the user id.
	account := email
	if account == "" {
		account = userID
	}
	res, err := h.svc.Enroll(c.Request.Context(), userID, account)
	if err != nil {
		if errors.Is(err, ErrAlreadyEnabled) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "already_enabled",
				"message": "MFA is already enabled — disable it first to re-enrol",
			})
			return
		}
		if h.logger != nil {
			h.logger.Error("mfa: enroll", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to enroll MFA",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			// qr_code_url is named so the admin UI can swap its
			// <img src={qr_code_url}> without a client-side change.
			"qr_code_url":  res.QRDataURL,
			"otpauth_url":  res.OtpauthURL,
			"secret":       res.Secret,
		},
	})
}

type verifyRequest struct {
	Code string `json:"code" binding:"required"`
}

func (h *Handler) verify(c *gin.Context) {
	userID, _, ok := userFromHeaders(c)
	if !ok {
		return
	}
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "code is required",
		})
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "MFA service not configured",
		})
		return
	}
	err := h.svc.Verify(c.Request.Context(), userID, req.Code)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": true}})
	case errors.Is(err, ErrNotEnrolled):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "not_enrolled",
			"message": "start enrolment before verifying",
		})
	case errors.Is(err, ErrInvalidCode):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_code",
			"message": "that code is not valid — please try again",
		})
	default:
		if h.logger != nil {
			h.logger.Error("mfa: verify", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to verify MFA code",
		})
	}
}

func (h *Handler) disable(c *gin.Context) {
	userID, _, ok := userFromHeaders(c)
	if !ok {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "MFA service not configured",
		})
		return
	}
	if err := h.svc.Disable(c.Request.Context(), userID); err != nil {
		if h.logger != nil {
			h.logger.Error("mfa: disable", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to disable MFA",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": false}})
}
