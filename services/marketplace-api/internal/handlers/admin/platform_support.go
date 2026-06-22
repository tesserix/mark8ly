// Package admin — platform_support.go: PlatformSupportHandler exposes the
// mobile admin merchant→platform support-chat routes, bridging to otto's
// fixed "platform" tenant (the same surface the web admin PlatformChat
// uses). A Tesserix employee picks the chat up from the platform-support
// inbox. The merchant's own tenant is carried as X-Client-Tenant-* so the
// agent can see which merchant is talking.
package admin

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/ottobridge"
	"github.com/mark8ly/marketplace-api/internal/ottoclient"
)

// Platform support pins to a single deterministic tenant/store on otto:
// every merchant talks to the same Tesserix support surface, so there's no
// per-merchant store row to invent.
const (
	platformTenantID = "platform"
	platformStoreID  = "default"
)

// PlatformSupportHandler exposes the mobile admin /platform-support/* routes.
type PlatformSupportHandler struct {
	bridge *ottobridge.Bridge
}

// NewPlatformSupportHandler builds the handler. otto may be nil (not wired
// in local dev) — every route then returns 503.
func NewPlatformSupportHandler(otto *ottoclient.Client, wsPublicBase string, logger *slog.Logger) *PlatformSupportHandler {
	return &PlatformSupportHandler{bridge: ottobridge.New(otto, wsPublicBase, logger)}
}

// Register mounts the platform-support routes onto g. The group MUST
// already have the admin GIP bearer auth (sets "user_id" + "tenant_id").
func (h *PlatformSupportHandler) Register(g *gin.RouterGroup) {
	h.bridge.Mount(g, platformResolver)
}

// platformResolver pins the otto scope to the platform tenant and forwards
// the admin's identity. The admin is otto's "customer" here; their own
// (merchant) tenant rides along as X-Client-Tenant-Id so the platform
// agent sees who raised the chat.
func platformResolver(c *gin.Context) (ottoclient.ForwardScope, ottoclient.ForwardIdentity, bool) {
	return ottoclient.ForwardScope{TenantID: platformTenantID, StoreID: platformStoreID},
		ottoclient.ForwardIdentity{
			UserID:         c.GetString("user_id"),
			ClientTenantID: c.GetString("tenant_id"),
		},
		true
}
