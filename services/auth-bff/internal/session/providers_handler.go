package session

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LinkedProvider is a single sign-in method bound to a user.
//
// ProviderID is the vocabulary the linked-providers panel keys on —
// "password", "google.com", "apple.com" — not a Zitadel enum and not a
// raw IDP id, except when the deployment has not named that IDP (see
// zitadellogin.providerIDForIDP: an unrecognised provider still renders
// under its raw id rather than vanishing from a list the merchant uses
// to audit account access).
type LinkedProvider struct {
	ProviderID string `json:"provider_id"`     // "password" or "google.com" or "apple.com"
	Email      string `json:"email,omitempty"` // empty when the provider asserted none
}

type providersResponse struct {
	Providers []LinkedProvider `json:"providers"`
}

// getMyProviders returns the linked sign-in methods for the current
// admin user, for the /settings/security page in the admin app.
//
// Reads the m8_session cookie for the user id and asks the configured
// resolver — zitadellogin.Client.UserLinkedProviders in every real
// deployment, bound in cmd/server/main.go. This handler holds no
// identity-provider knowledge of its own, which is what let the GIP
// Identity Toolkit accounts:lookup it used to make be swapped out
// without the admin app changing: the response shape
// ({"data":{"providers":[{"provider_id","email"}]}}) is unchanged.
func (h *Handler) getMyProviders(c *gin.Context) {
	s, err := h.mgr.Read(c.Request)
	if err != nil || s == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_session",
			"message": "no active admin session",
		})
		return
	}

	// 503, never an empty list: "we cannot answer" and "you have no
	// sign-in methods" must not look the same to the security page.
	if h.providers == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "providers lookup is not configured",
		})
		return
	}

	found, err := h.providers.LinkedProviders(c.Request.Context(), s.UID)
	if err != nil {
		// The uid and the error only — a provider list is personal data
		// and must not reach the log stream.
		if h.logger != nil {
			h.logger.Info("providers lookup unavailable", "err", err, "user_id", s.UID)
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
