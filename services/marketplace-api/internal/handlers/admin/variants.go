// Package admin — variants.go: HTTP handler for the admin variant
// quick-PATCH surface. Scope is narrow: price, stock, sku/barcode,
// weight, position, inventory policy/threshold. Currency change is
// rejected with currency_change_forbidden. Option-value matrix
// changes go through the product PATCH flow, not this endpoint.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// VariantHandler bundles dependencies for the variant endpoints.
type VariantHandler struct {
	svc    *product.Service
	logger *slog.Logger
}

// NewVariantHandler constructs a VariantHandler.
func NewVariantHandler(svc *product.Service, logger *slog.Logger) *VariantHandler {
	return &VariantHandler{svc: svc, logger: logger}
}

// Patch handles PATCH /admin/stores/:storeId/products/:id/variants/:variantId.
func (h *VariantHandler) Patch(c *gin.Context) {
	storeID := c.Param("storeId")
	productID := c.Param("id")
	variantID := c.Param("variantId")
	tenantID := c.GetString("tenant_id")

	var req UpdateVariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := toServiceUpdateVariantBasics(req, productID, variantID, storeID, tenantID)
	v, err := h.svc.UpdateVariantBasics(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminVariantResponse(v))
}
