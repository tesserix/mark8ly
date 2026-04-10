// Package admin — stores.go: handler for the admin stores listing.
// Used by the CopyToStoreDialog to enumerate stores the authenticated
// user can manage. The endpoint sits outside the /stores/:storeId group
// because it returns ALL stores for the tenant, not a single store.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// StoresHandler bundles dependencies for the admin stores endpoints.
type StoresHandler struct {
	repo   stores.Repository
	logger *slog.Logger
}

// NewStoresHandler constructs a StoresHandler.
func NewStoresHandler(repo stores.Repository, logger *slog.Logger) *StoresHandler {
	return &StoresHandler{repo: repo, logger: logger}
}

// AdminStoreResponse is the wire DTO for the stores list endpoint.
type AdminStoreResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	CountryCode  string `json:"country_code"`
	CurrencyCode string `json:"currency_code"`
	Status       string `json:"status"`
}

// List handles GET /admin/stores — returns all active stores for the
// caller's tenant. The FGA middleware upstream guarantees the user has
// at least staff-level access to the tenant before this handler runs.
func (h *StoresHandler) List(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing tenant context",
		})
		return
	}

	rows, err := h.repo.ListForTenant(c.Request.Context(), tenantID)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("list stores for tenant", "tenant_id", tenantID, "err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal server error",
		})
		return
	}

	resp := make([]AdminStoreResponse, 0, len(rows))
	for _, s := range rows {
		resp = append(resp, AdminStoreResponse{
			ID:           s.ID,
			Name:         s.Name,
			Slug:         s.Slug,
			CountryCode:  s.CountryCode,
			CurrencyCode: s.CurrencyCode,
			Status:       s.Status,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}
