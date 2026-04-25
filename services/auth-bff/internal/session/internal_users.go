package session

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UserEraser is what /internal/users/:id needs from downstream state
// stores: revoke every active session and wipe MFA enrolment. Kept
// narrow so the handler doesn't reach into the usersessions or
// usermfa packages directly.
type UserEraser interface {
	RevokeAllForUser(ctx context.Context, userID string) error
}

// MFAEraser wipes a user's TOTP enrolment, returning nil if there
// was nothing to remove.
type MFAEraser interface {
	Disable(ctx context.Context, userID string) error
}

// InternalUsersHandler exposes service-to-service endpoints that
// marketplace-api calls when the merchant asks to "reset my profile".
// Guarded by a shared X-Internal-Auth header — never mounted on a
// public route group.
type InternalUsersHandler struct {
	sessions UserEraser
	mfa      MFAEraser
	secret   string
	logger   *slog.Logger
}

// NewInternalUsersHandler constructs the handler. secret is the
// expected value of the X-Internal-Auth header; an empty secret
// disables the endpoint entirely (every request returns 503) so
// misconfigured deploys fail closed.
func NewInternalUsersHandler(
	sessions UserEraser,
	mfa MFAEraser,
	secret string,
	logger *slog.Logger,
) *InternalUsersHandler {
	return &InternalUsersHandler{
		sessions: sessions,
		mfa:      mfa,
		secret:   secret,
		logger:   logger,
	}
}

// Register mounts DELETE /internal/users/:id onto the given router group.
func (h *InternalUsersHandler) Register(r *gin.RouterGroup) {
	r.DELETE("/users/:id", h.deleteUser)
}

func (h *InternalUsersHandler) deleteUser(c *gin.Context) {
	if h.secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "internal auth secret is not configured",
		})
		return
	}
	if !constantTimeEqual(c.GetHeader("X-Internal-Auth"), h.secret) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing or invalid internal auth",
		})
		return
	}
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "user id is required",
		})
		return
	}

	ctx := c.Request.Context()

	// Revoke sessions first: even if MFA teardown fails, the user is
	// already signed out of every device.
	if h.sessions != nil {
		if err := h.sessions.RevokeAllForUser(ctx, userID); err != nil {
			if h.logger != nil {
				h.logger.Error("internal users: revoke sessions", "err", err, "user_id", userID)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "failed to revoke sessions",
			})
			return
		}
	}

	if h.mfa != nil {
		if err := h.mfa.Disable(ctx, userID); err != nil {
			if h.logger != nil {
				h.logger.Error("internal users: disable mfa", "err", err, "user_id", userID)
			}
			// Not fatal — the user is already logged out. Log and continue.
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"user_id": userID, "erased": true},
	})
}

func constantTimeEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}
