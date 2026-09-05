package admin

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/teamproxy"
)

// TenantLister is the narrow slice of teamproxy.Client this handler needs.
type TenantLister interface {
	ListMyTenants(ctx context.Context, id string) ([]teamproxy.TenantMembership, error)
}

// MobileMyTenantsHandler serves GET /mobile/admin/me/tenants: the tenants
// the authenticated caller belongs to.
//
// # Why this route exists, and why it is NOT tenant-gated (#686)
//
// It is the only mobile admin route mounted outside RequireBoundTenant,
// and that is the entire point. A GIP token carried a tenant claim, so
// the client never had to ask. A Zitadel token carries none, tenancy
// instead comes from the client stating X-Acting-Tenant-Id — and nothing
// told the client what to put there. Every other route 404s a caller with
// no bound tenant, so gating this one too would deadlock the flow: you
// would need a tenant in order to discover your tenant.
//
// Authentication is NOT relaxed, only the tenant gate. The identity comes
// from the verified bearer token's user_id, never from a query parameter
// or header — otherwise this would be an arbitrary-identity membership
// lookup, which is exactly the property that keeps platform-api's own
// (unauthenticated) endpoint in-cluster.
type MobileMyTenantsHandler struct {
	lister TenantLister
	log    *slog.Logger
}

func NewMobileMyTenantsHandler(lister TenantLister, log *slog.Logger) *MobileMyTenantsHandler {
	return &MobileMyTenantsHandler{lister: lister, log: log}
}

// List responds with {"data": [...]}.
//
// An upstream failure is 502, deliberately NOT an empty list: the client
// routes "zero tenants" to the finish-onboarding screen, so degrading a
// platform-api outage into [] would tell an established merchant they
// have no store.
func (h *MobileMyTenantsHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		// bearerAuth always sets user_id, so this is unreachable in the
		// mounted chain; fail closed rather than look up "".
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized", "message": "bearer token required",
		})
		return
	}

	tenants, err := h.lister.ListMyTenants(c.Request.Context(), userID)
	if err != nil {
		if h.log != nil {
			h.log.Error("mobile: tenant discovery failed", "user_id", userID, "error", err)
		}
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "platform_unavailable", "message": "could not look up your stores",
		})
		return
	}

	// Never null: the client parses this with a strict schema, and a null
	// would fail it where an empty list is a legitimate answer.
	if tenants == nil {
		tenants = []teamproxy.TenantMembership{}
	}
	c.JSON(http.StatusOK, gin.H{"data": tenants})
}
