// Package storefront — support.go: MobileSupportHandler exposes the mobile
// storefront support-chat routes (customer→merchant), bridging to the otto
// service via the shared ottobridge. The web storefront reaches otto
// through a Next.js proxy; mobile authenticates with a GIP bearer token, so
// this is the server-side equivalent — it resolves the store scope + the
// verified customer identity and lets the bridge do the rest.
package storefront

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/ottobridge"
	"github.com/mark8ly/marketplace-api/internal/ottoclient"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// MobileSupportHandler exposes the mobile storefront /support/* routes.
type MobileSupportHandler struct {
	bridge *ottobridge.Bridge
}

// NewMobileSupportHandler builds the handler. otto may be nil (otto not
// wired in local dev) — every route then returns 503. wsPublicBase is the
// public WebSocket origin the app dials, e.g. "wss://api.mark8ly.com".
func NewMobileSupportHandler(otto *ottoclient.Client, wsPublicBase string, logger *slog.Logger) *MobileSupportHandler {
	return &MobileSupportHandler{bridge: ottobridge.New(otto, wsPublicBase, logger)}
}

// Register mounts the support routes onto g. The group MUST already have
// StoreContext (so "store" is set) and the customer-identity middleware
// applied.
func (h *MobileSupportHandler) Register(g *gin.RouterGroup) {
	h.bridge.Mount(g, storefrontResolver)
}

// storefrontResolver maps the resolved store + verified customer onto the
// otto scope/identity. A non-empty UserID makes otto skip the anonymous
// OTP step for the logged-in customer.
func storefrontResolver(c *gin.Context) (ottoclient.ForwardScope, ottoclient.ForwardIdentity, bool) {
	v, exists := c.Get("store")
	s, isStore := v.(*stores.Store)
	if !exists || !isStore || s == nil {
		c.JSON(500, gin.H{"error": "store_unresolved"})
		return ottoclient.ForwardScope{}, ottoclient.ForwardIdentity{}, false
	}
	return ottoclient.ForwardScope{TenantID: s.TenantID, StoreID: s.ID},
		ottoclient.ForwardIdentity{
			UserID:    c.GetString(CustomerGipUIDKey),
			UserEmail: c.GetString(CustomerEmailKey),
		},
		true
}
