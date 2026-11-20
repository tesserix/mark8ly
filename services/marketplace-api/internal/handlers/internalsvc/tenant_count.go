package internalsvc

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// countFn matches tenantpurge.Count's signature minus the *gorm.DB, flattened
// to table → rows, so tests can inject a fake with no DB dependency.
type countFn func(ctx context.Context, tenantID string, storeIDs []string) (map[string]int64, error)

// TenantCountHandler answers POST /internal/tenants/:tenantID/purge-preview:
// what a tenant's purge WOULD delete, per table, deleting nothing. platform-api
// calls it just before a merchant self-serve delete so the "account deleted"
// operator notice can say what the merchant had built — the purge itself
// erases every row (audit_logs included) that could answer that afterwards.
type TenantCountHandler struct {
	count countFn
}

// NewTenantCountHandler constructs a TenantCountHandler. In production,
// main.go binds count to tenantpurge.Count closed over the service's *gorm.DB.
func NewTenantCountHandler(count countFn) *TenantCountHandler {
	return &TenantCountHandler{count: count}
}

// Count handles POST /internal/tenants/:tenantID/purge-preview.
func (h *TenantCountHandler) Count(c *gin.Context) {
	var req tenantPurgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}
	tables, err := h.count(c.Request.Context(), c.Param("tenantID"), req.StoreIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

// Register mounts the endpoint behind the same X-Internal-Auth secret as the purge.
func (h *TenantCountHandler) Register(group *gin.RouterGroup, internalSecret string) {
	group.POST("/tenants/:tenantID/purge-preview", RequireInternalAuth(internalSecret), h.Count)
}
