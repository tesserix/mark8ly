package session

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/usersessions"
)

// FGAChecker is the subset of the authz.Client interface that the
// session handler needs for Phase P's switch-tenant endpoint. Kept
// narrow so we can fake it in tests without pulling in the whole
// OpenFGA SDK.
type FGAChecker interface {
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)
}

// Handler is the HTTP layer for session introspection and logout. Lives
// in the same package as Manager because these routes are the only two
// callers of Read/Clear outside the autologin mint path.
type Handler struct {
	mgr      *Manager
	fga      FGAChecker // may be nil in dev if OpenFGA init failed
	registry *usersessions.Repository
	logger   *slog.Logger
}

// NewHandler constructs a Handler over the given Manager.
//
// fga may be nil — dev without OpenFGA degrades the switch-tenant
// endpoint to "allow any target id", consistent with the rest of the
// platform-api Phase O/P fallbacks. Never deploy to prod with fga nil.
// registry and logger are optional — when nil, the active-sessions UI
// simply shows no sessions.
func NewHandler(mgr *Manager, fga FGAChecker) *Handler {
	return &Handler{mgr: mgr, fga: fga}
}

// WithRegistry wires the user_sessions registry + logger. Nil-safe:
// the list/revoke endpoints return empty / no-op when registry is nil.
func (h *Handler) WithRegistry(r *usersessions.Repository, logger *slog.Logger) *Handler {
	h.registry = r
	h.logger = logger
	return h
}

// Register mounts the session routes onto the given gin.RouterGroup.
//
//	GET  /auth/session          — returns { user_id, email, tenant_id }
//	POST /auth/logout           — clears the session cookie
//	POST /auth/switch-tenant    — re-mints the cookie with a new tenant_id
//	GET  /api/v1/sessions       — list active sessions for the current user
//	DELETE /api/v1/sessions/:id — revoke a single session row
//
// The cookie IS the session for auth purposes; the list/revoke endpoints
// read/write the side-registry used by the admin "Active sessions" UI.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.GET("/session", h.getSession)
	r.POST("/logout", h.logout)
	r.POST("/switch-tenant", h.switchTenant)
	r.POST("/switch-store", h.switchStore)
}

// RegisterAPI mounts the /api/v1 subset of session routes. Kept separate
// from Register so main.go can pick the mount path per route group.
func (h *Handler) RegisterAPI(r *gin.RouterGroup) {
	r.GET("/sessions", h.listSessions)
	r.DELETE("/sessions/:id", h.revokeSession)
}

// sessionResponse is the JSON shape handed back to authenticated
// callers. Mirrors the public subset of Session — no internal-only
// fields (issued_at, expires_at) bleed through.
type sessionResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
	StoreID  string `json:"store_id,omitempty"`
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
			StoreID:  s.StoreID,
		},
	})
}

// logout clears the session cookie and revokes every active registry
// row for the user. Always returns 200 — logging out when you're
// already logged out is not an error.
func (h *Handler) logout(c *gin.Context) {
	// Best-effort revoke before clearing the cookie so we can still read
	// the user id from the existing session.
	if h.registry != nil {
		if existing, err := h.mgr.Read(c.Request); err == nil && existing != nil {
			if err := h.registry.RevokeAllForUser(c.Request.Context(), existing.UID); err != nil && h.logger != nil {
				h.logger.Warn("usersessions: revoke-all on logout failed", "err", err, "user_id", existing.UID)
			}
		}
	}
	h.mgr.Clear(c.Writer)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"logged_out": true}})
}

// sessionRow is the wire shape for a single session in the list response.
// Field names match AccountSession in apps/admin/lib/api/settings-tier2-api.ts.
type sessionRow struct {
	ID         string `json:"id"`
	Device     string `json:"device"`
	IPAddress  string `json:"ip_address"`
	LastActive string `json:"last_active"`
	Current    bool   `json:"current"`
}

// listSessions handles GET /api/v1/sessions — returns every non-revoked
// session row for the requesting user. User is identified by the
// X-User-Id header forwarded by marketplace-api (which itself extracts
// it from the session cookie).
func (h *Handler) listSessions(c *gin.Context) {
	userID := c.GetHeader("X-User-Id")
	if userID == "" {
		// Fall back to the cookie for direct auth-bff calls.
		if s, err := h.mgr.Read(c.Request); err == nil && s != nil {
			userID = s.UID
		}
	}
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_session",
			"message": "no user context on request",
		})
		return
	}
	if h.registry == nil {
		c.JSON(http.StatusOK, gin.H{"data": []sessionRow{}})
		return
	}

	rows, err := h.registry.ListForUser(c.Request.Context(), userID)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("usersessions: list failed", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to list sessions",
		})
		return
	}

	out := make([]sessionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, sessionRow{
			ID:         r.ID,
			Device:     r.Device,
			IPAddress:  r.IPAddress,
			LastActive: r.LastActiveAt.UTC().Format(time.RFC3339),
			// "Current" marker deferred: needs session_id inside the
			// JWT cookie payload to correlate registry row → request.
			Current: false,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// revokeSession handles DELETE /api/v1/sessions/:id — marks one row
// revoked. User scoping is enforced inside the repository.
func (h *Handler) revokeSession(c *gin.Context) {
	userID := c.GetHeader("X-User-Id")
	if userID == "" {
		if s, err := h.mgr.Read(c.Request); err == nil && s != nil {
			userID = s.UID
		}
	}
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_session",
			"message": "no user context on request",
		})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "session id is required",
		})
		return
	}
	if h.registry == nil {
		c.Status(http.StatusNoContent)
		return
	}
	err := h.registry.Revoke(c.Request.Context(), id, userID)
	switch {
	case err == nil:
		c.Status(http.StatusNoContent)
	case errors.Is(err, usersessions.ErrNotOwned):
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "session does not belong to the current user",
		})
	default:
		if h.logger != nil {
			h.logger.Error("usersessions: revoke failed", "err", err, "user_id", userID, "session_id", id)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to revoke session",
		})
	}
}

// switchTenantRequest is the body shape for POST /auth/switch-tenant.
type switchTenantRequest struct {
	TenantID string `json:"tenant_id"`
}

// switchTenant re-mints the session cookie against a different tenant.
//
// Phase P: the admin app calls this from the tenant switcher dropdown
// and from the accept-invite flow to move the user onto a newly
// joined tenant. The flow is:
//
//  1. Read and validate the existing session cookie.
//  2. Verify the user has an FGA `member` relation on the target tenant.
//     (Fails closed: no cookie → 401; not a member → 403.)
//  3. Mint a new session with the same uid/email but the new tenant_id,
//     preserving the original ExpiresAt so switching doesn't silently
//     extend the user's session window.
//
// The cookie is re-set in-place via Mint, which Set-Cookies the browser.
func (h *Handler) switchTenant(c *gin.Context) {
	existing, err := h.mgr.Read(c.Request)
	if err != nil || existing == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "invalid_session",
			"message": "no active session",
		})
		return
	}

	var req switchTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "tenant_id is required",
		})
		return
	}

	// Authorize the switch against OpenFGA. The `member` relation is
	// the derived union over all four roles, so any non-empty role on
	// the target tenant passes.
	if h.fga != nil {
		allowed, err := h.fga.CheckMembership(c.Request.Context(), existing.UID, req.TenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "authz_check_failed",
				"message": "authorization check failed",
			})
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "you do not have access to this tenant",
			})
			return
		}
	}

	// Mint the new session. Preserve ExpiresAt so a user bouncing
	// between tenants every minute doesn't get a perpetually
	// refreshed cookie. StoreID is deliberately cleared: store ids
	// are tenant-scoped, so switching tenants resets "current
	// store" and the admin resolves a new default on next render.
	now := time.Now()
	next := Session{
		UID:       existing.UID,
		Email:     existing.Email,
		TenantID:  req.TenantID,
		IssuedAt:  now,
		ExpiresAt: existing.ExpiresAt,
	}
	if err := h.mgr.Mint(c.Writer, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "session_mint_failed",
			"message": "failed to issue new session",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": sessionResponse{
			UserID:   next.UID,
			Email:    next.Email,
			TenantID: next.TenantID,
		},
	})
}

// switchStoreRequest is the body shape for POST /auth/switch-store.
type switchStoreRequest struct {
	StoreID string `json:"store_id"`
}

// switchStore re-mints the session cookie with a different store id
// under the same tenant. Does not run an FGA check: store access is
// derived from tenant membership via the Phase Q DSL
// (`member from parent` on the store type). Any user with a tenant
// role automatically has access to every store under that tenant.
// Phase R will add a real Check against `can_view_store` when
// per-store role grants exist.
func (h *Handler) switchStore(c *gin.Context) {
	existing, err := h.mgr.Read(c.Request)
	if err != nil || existing == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "invalid_session",
			"message": "no active session",
		})
		return
	}

	var req switchStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.StoreID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "store_id is required",
		})
		return
	}

	now := time.Now()
	next := Session{
		UID:       existing.UID,
		Email:     existing.Email,
		TenantID:  existing.TenantID,
		StoreID:   req.StoreID,
		IssuedAt:  now,
		ExpiresAt: existing.ExpiresAt,
	}
	if err := h.mgr.Mint(c.Writer, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "session_mint_failed",
			"message": "failed to issue new session",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": sessionResponse{
			UserID:   next.UID,
			Email:    next.Email,
			TenantID: next.TenantID,
			StoreID:  next.StoreID,
		},
	})
}
