// Package admin — mobile_account.go: mobile account-deletion handler.
// Proxies to platform-api's internal tenant-account endpoint (via
// internal/teamproxy) so the mobile admin app can offer "delete my
// account" (Apple App Store requires this for any app with sign-up).
// Named MobileAccountHandler (not AccountHandler) because the web admin
// already owns that name in account.go for the unrelated
// profile-reset/MFA/sessions handler.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/teamproxy"
)

// MobileAccountHandler serves the mobile DELETE /mobile/admin/account
// endpoint. Tenant-scoped: tenant_id + actor UID come from the GIP bearer
// token (there is no :storeId on this route).
type MobileAccountHandler struct {
	client *teamproxy.Client
	logger *slog.Logger
}

// NewMobileAccountHandler constructs a MobileAccountHandler. client is required.
func NewMobileAccountHandler(client *teamproxy.Client, logger *slog.Logger) *MobileAccountHandler {
	return &MobileAccountHandler{client: client, logger: logger}
}

// Delete handles DELETE /mobile/admin/account — removes the calling user's
// account. platform-api is authoritative on the owner-vs-staff distinction:
// an owner triggers tenant teardown, staff only lose their membership. Any
// tenant member may call this (no role gate here — Apple requires the
// deletion path to work for staff accounts too).
func (h *MobileAccountHandler) Delete(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	actorUID := c.GetString("user_id")
	if err := h.client.DeleteTenantAccount(c.Request.Context(), tenantID, actorUID); err != nil {
		respondProxyErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}
