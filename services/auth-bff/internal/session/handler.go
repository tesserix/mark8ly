package session

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler is the HTTP layer for session introspection and logout. Lives
// in the same package as Manager because these routes are the only two
// callers of Read/Clear outside the autologin mint path.
type Handler struct {
	mgr *Manager
}

// NewHandler constructs a Handler over the given Manager.
func NewHandler(mgr *Manager) *Handler {
	return &Handler{mgr: mgr}
}

// Register mounts the session routes onto the given gin.RouterGroup.
//
//	GET  /auth/session   — returns { user_id, email, tenant_id } from the
//	                        cookie, or 401 if the cookie is missing /
//	                        expired / tampered
//	POST /auth/logout    — clears the session cookie
//
// Both routes are deliberately simple: no DB lookup, no FGA call, no
// tenant row resolution. The cookie IS the session; anything that needs
// more than what's encoded in the cookie should call platform-api or
// OpenFGA directly.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.GET("/session", h.getSession)
	r.POST("/logout", h.logout)
}

// sessionResponse is the JSON shape handed back to authenticated
// callers. Mirrors the public subset of Session — no internal-only
// fields (issued_at, expires_at) bleed through.
type sessionResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
}

// getSession reads the session cookie and returns its payload. The
// canonical "am I logged in?" probe used by downstream apps (admin,
// storefront) before rendering any authenticated surface.
func (h *Handler) getSession(c *gin.Context) {
	s, err := h.mgr.Read(c.Request)
	if err != nil {
		// Tampered or expired cookies are both "please log in again."
		if errors.Is(err, ErrInvalidSession) || errors.Is(err, ErrExpiredSession) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_session",
				"message": "session cookie is invalid or expired",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to read session",
		})
		return
	}
	if s == nil {
		// No cookie on the request at all. Distinct from an invalid one
		// so the client can tell "never logged in" from "session died."
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_session",
			"message": "no session cookie on request",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": sessionResponse{
			UserID:   s.UID,
			Email:    s.Email,
			TenantID: s.TenantID,
		},
	})
}

// logout clears the session cookie. Always returns 200 — logging out
// when you're already logged out is not an error, and returning the
// same status for both cases saves the client a branch.
func (h *Handler) logout(c *gin.Context) {
	h.mgr.Clear(c.Writer)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"logged_out": true}})
}
