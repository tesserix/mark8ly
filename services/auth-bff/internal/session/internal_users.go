package session

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/internalauth"
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

// DisplayNameResolver resolves the human name the identity provider holds
// for a user id. Implemented by zitadellogin.Client.UserDisplayName.
//
// A user with no name a person actually supplied returns ("", nil) — a
// real answer, not an error. See that method's doc for why a placeholder
// profile is reported as no name at all.
type DisplayNameResolver interface {
	UserDisplayName(ctx context.Context, userID string) (string, error)
}

// LinkedProvidersResolver lists a user's sign-in methods.
//
// The identity-provider ids that turn a federated link into "google.com"
// rather than a raw Zitadel id are deployment config, so they are bound
// by whoever constructs the resolver (see cmd/server/main.go) rather than
// travelling through this handler. That keeps this package free of any
// dependency on the Zitadel client.
//
// A user with no methods returns an empty slice and no error. The element
// type is the same LinkedProvider providers_handler.go already defines,
// so both the merchant and customer surfaces answer in one vocabulary:
// "password", "google.com", "apple.com" — the names
// packages/ui/src/auth/linked-providers-panel.tsx keys on.
type LinkedProvidersResolver interface {
	LinkedProviders(ctx context.Context, userID string) ([]LinkedProvider, error)
}

// LinkedProvidersFunc adapts a plain function to LinkedProvidersResolver.
type LinkedProvidersFunc func(ctx context.Context, userID string) ([]LinkedProvider, error)

// LinkedProviders implements LinkedProvidersResolver.
func (f LinkedProvidersFunc) LinkedProviders(ctx context.Context, userID string) ([]LinkedProvider, error) {
	return f(ctx, userID)
}

// InternalUsersHandler exposes service-to-service endpoints that
// marketplace-api calls when the merchant asks to "reset my profile".
// Guarded by a shared X-Internal-Auth header — never mounted on a
// public route group.
type InternalUsersHandler struct {
	sessions     UserEraser
	mfa          MFAEraser
	displayNames DisplayNameResolver
	providers    LinkedProvidersResolver
	secret       string
	logger       *slog.Logger
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

// WithDisplayNames wires the identity-provider name lookup that backs
// GET /internal/users/:id/display-name. Optional: leaving it unset (the
// deployment has no Zitadel client, so there is nothing to ask) makes
// that endpoint answer with an empty name rather than an error, because
// "this deployment holds no name for that user" is the truth and the
// caller's only correct response to either is the same blank seed.
func (h *InternalUsersHandler) WithDisplayNames(r DisplayNameResolver) *InternalUsersHandler {
	h.displayNames = r
	return h
}

// WithLinkedProviders wires the sign-in-method lookup that backs
// GET /internal/users/:id/providers. Optional, and unset means the
// endpoint answers 503 rather than an empty list: unlike a display name,
// "no providers" is not a safe blank. A customer reading their security
// page would see an empty "Linked sign-in methods" list and conclude
// their Google link had been removed, so a deployment that cannot answer
// must say so instead of answering wrongly.
func (h *InternalUsersHandler) WithLinkedProviders(r LinkedProvidersResolver) *InternalUsersHandler {
	h.providers = r
	return h
}

// Register mounts the internal user endpoints onto the given router group.
func (h *InternalUsersHandler) Register(r *gin.RouterGroup) {
	r.DELETE("/users/:id", h.deleteUser)
	r.GET("/users/:id/display-name", h.displayName)
	r.GET("/users/:id/providers", h.linkedProviders)
}

// displayName handles GET /internal/users/:id/display-name.
//
// marketplace-api calls this once, when it first creates a merchant's
// user_profiles row, so the name the merchant already gave Google or
// Apple lands in their admin profile without them retyping it. It is
// deliberately NOT a per-request read: both IDPs are configured
// isAutoUpdate:false because Apple returns a name only on the first
// authorization and Google would otherwise overwrite an edited name at
// every sign-in.
func (h *InternalUsersHandler) displayName(c *gin.Context) {
	if !h.authorize(c) {
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

	if h.displayNames == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{"user_id": userID, "display_name": ""},
		})
		return
	}

	name, err := h.displayNames.UserDisplayName(c.Request.Context(), userID)
	if err != nil {
		// Deliberately logs the uid and the error only — a display name
		// is personal data and must not reach the log stream.
		if h.logger != nil {
			h.logger.Info("internal users: display name unavailable", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "upstream_unavailable",
			"message": "could not resolve display name",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"user_id": userID, "display_name": name},
	})
}

// linkedProviders handles GET /internal/users/:id/providers.
//
// The storefront's /api/account/providers route calls this to render a
// customer's "Linked sign-in methods" panel (#787). It replaces a direct
// Identity Toolkit accounts:lookup the storefront pod used to make; the
// Zitadel credential that answers it lives only here, which is why this
// is a service-to-service hop rather than the storefront reading Zitadel
// itself.
//
// The response shape is byte-for-byte the one the storefront route
// already returned, so the browser-side consumer needed no change.
func (h *InternalUsersHandler) linkedProviders(c *gin.Context) {
	if !h.authorize(c) {
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

	if h.providers == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "linked provider lookup is not configured",
		})
		return
	}

	found, err := h.providers.LinkedProviders(c.Request.Context(), userID)
	if err != nil {
		// The uid and the error only. A provider list is personal data
		// and must not reach the log stream.
		if h.logger != nil {
			h.logger.Info("internal users: linked providers unavailable", "err", err, "user_id", userID)
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "upstream_unavailable",
			"message": "could not resolve linked providers",
		})
		return
	}

	// Non-nil so an account with no methods marshals as [] and not null
	// — the consumer maps over it unconditionally.
	if found == nil {
		found = []LinkedProvider{}
	}
	c.JSON(http.StatusOK, gin.H{"data": providersResponse{Providers: found}})
}

// authorize enforces the shared-secret guard every route in this handler
// sits behind. It writes the response and returns false on refusal.
func (h *InternalUsersHandler) authorize(c *gin.Context) bool {
	if h.secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "internal auth secret is not configured",
		})
		return false
	}
	if !internalauth.Equal(c.GetHeader(internalauth.Header), h.secret) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing or invalid internal auth",
		})
		return false
	}
	return true
}

func (h *InternalUsersHandler) deleteUser(c *gin.Context) {
	if !h.authorize(c) {
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
