package internalsvc

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// purgeFn matches tenantpurge.Purge's signature minus the *gorm.DB
// parameter (bound by main.go via a closure), so tests can inject a fake
// with no DB dependency at all.
type purgeFn func(ctx context.Context, tenantID string, storeIDs []string) error

// TenantPurgeHandler answers POST /internal/tenants/:tenantID/purge — the
// HTTP trigger for tenantpurge.Purge, called by platform-api's outbox
// drainer as the destructive step of the tenant hard-delete flow.
//
// purge is injected (rather than holding a *gorm.DB directly) so unit
// tests can exercise routing/validation/status-code behavior without a
// live database — see tenant_purge_test.go.
type TenantPurgeHandler struct {
	purge purgeFn
}

// NewTenantPurgeHandler constructs a TenantPurgeHandler. In production,
// main.go binds purge to tenantpurge.Purge closed over the service's
// *gorm.DB; tests inject a fake.
func NewTenantPurgeHandler(purge purgeFn) *TenantPurgeHandler {
	return &TenantPurgeHandler{purge: purge}
}

// tenantPurgeRequest is the wire body for POST /internal/tenants/:tenantID/purge.
type tenantPurgeRequest struct {
	StoreIDs []string `json:"store_ids"`
}

// Purge handles POST /internal/tenants/:tenantID/purge.
//
// tenantpurge.Purge is idempotent (safe to replay), so a second call for
// an already-purged tenant is expected to return nil and this handler
// answers 200 either way. A genuine purge error returns 500 so the
// platform-api outbox drainer retries the delivery.
func (h *TenantPurgeHandler) Purge(c *gin.Context) {
	tenantID := c.Param("tenantID")

	var req tenantPurgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": err.Error(),
		})
		return
	}

	if err := h.purge(c.Request.Context(), tenantID, req.StoreIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "purge_failed",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"purged": true})
}

// Register mounts the tenant purge endpoint onto the supplied /internal
// route group, gated by the shared X-Internal-Auth secret (matching the
// other internalsvc handlers' convention).
func (h *TenantPurgeHandler) Register(group *gin.RouterGroup, internalSecret string) {
	group.POST("/tenants/:tenantID/purge", RequireInternalAuth(internalSecret), h.Purge)
}
