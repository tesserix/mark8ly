package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// Handler is the HTTP entry point for the password-reset flow.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the routes. Only /internal is used here — these
// endpoints are called by the admin app's BFF layer, never by a
// browser directly, so they live alongside the other internal
// platform-api endpoints (tenants, stores, invitations).
func (h *Handler) Register(internal *gin.RouterGroup) {
	g := internal.Group("/auth/password-reset")
	{
		g.POST("/request", h.request)
		g.POST("/confirm", h.confirm)
	}
}

type requestBody struct {
	Email string `json:"email"`
}

// request handles POST /internal/auth/password-reset/request.
//
// Always returns 204 on behalf of the caller unless the upstream
// identity service is genuinely unreachable — this prevents callers
// from using the endpoint as an account-existence oracle.
func (h *Handler) request(c *gin.Context) {
	var body requestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "invalid request body",
		})
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "email is required",
		})
		return
	}

	if err := h.svc.RequestPasswordReset(c.Request.Context(), email); err != nil {
		// Rate limiting and upstream transient errors are user-visible
		// so the admin can show a friendly retry message. Everything
		// else is a server-side error.
		switch {
		case errors.Is(err, gipadmin.ErrTooManyAttempts):
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "too_many_attempts",
				"message": "Too many attempts, please try again shortly.",
			})
			return
		case errors.Is(err, gipadmin.ErrUnavailable),
			errors.Is(err, gipadmin.ErrUnauthenticated):
			if h.logger != nil {
				h.logger.Error("auth: password reset upstream failure",
					"err", err, "email", email)
			}
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "upstream_unavailable",
				"message": "We couldn't reach the identity service. Please try again in a moment.",
			})
			return
		default:
			if h.logger != nil {
				h.logger.Error("auth: password reset failed",
					"err", err, "email", email)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "internal server error",
			})
			return
		}
	}

	c.Status(http.StatusNoContent)
}

type confirmBody struct {
	OobCode     string `json:"oob_code"`
	NewPassword string `json:"new_password"`
}

// confirm handles POST /internal/auth/password-reset/confirm.
func (h *Handler) confirm(c *gin.Context) {
	var body confirmBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "invalid request body",
		})
		return
	}

	if err := h.svc.ConfirmPasswordReset(c.Request.Context(), body.OobCode, body.NewPassword); err != nil {
		switch {
		case errors.Is(err, gipadmin.ErrInvalidOobCode):
			c.JSON(http.StatusGone, gin.H{
				"error":   "invalid_oob_code",
				"message": "This reset link is invalid or has expired. Request a new one from the forgot-password page.",
			})
			return
		case errors.Is(err, gipadmin.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "weak_password",
				"message": "Please choose a stronger password — at least 8 characters.",
			})
			return
		case errors.Is(err, gipadmin.ErrUnavailable):
			if h.logger != nil {
				h.logger.Error("auth: reset confirm upstream failure", "err", err)
			}
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "upstream_unavailable",
				"message": "We couldn't reach the identity service. Please try again in a moment.",
			})
			return
		default:
			if h.logger != nil {
				h.logger.Error("auth: reset confirm failed", "err", err)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "internal server error",
			})
			return
		}
	}

	c.Status(http.StatusNoContent)
}
