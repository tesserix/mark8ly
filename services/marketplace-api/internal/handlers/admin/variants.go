// Package admin — variants.go: HTTP handler for the admin variant
// quick-PATCH surface. Scope is narrow: price, stock, sku/barcode,
// weight, position, inventory policy/threshold. Currency change is
// rejected with currency_change_forbidden. Option-value matrix
// changes go through the product PATCH flow, not this endpoint.
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// VariantHandler bundles dependencies for the variant endpoints.
type VariantHandler struct {
	svc    *product.Service
	db     *gorm.DB              // used for low-stock crossing pre-read
	notify *notification.Service // optional — nil-safe
	logger *slog.Logger
}

// NewVariantHandler constructs a VariantHandler.
func NewVariantHandler(svc *product.Service, logger *slog.Logger) *VariantHandler {
	return &VariantHandler{svc: svc, logger: logger}
}

// WithNotifier attaches the notification service + db handle so PATCHes
// that drive inventory below threshold fire low_stock notifications.
// Nil-safe: without a notifier the crossing-detection no-ops.
func (h *VariantHandler) WithNotifier(db *gorm.DB, svc *notification.Service) *VariantHandler {
	h.db = db
	h.notify = svc
	return h
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

	// Low-stock crossing detection: snapshot the pre-update inventory
	// quantity so we can tell after the patch whether this write crossed
	// the threshold (as opposed to sitting below it all along). Only done
	// when the patch actually touches inventory_quantity and a notifier
	// is wired. Failures here are non-fatal — the PATCH still proceeds.
	var preQty int
	preQtyKnown := false
	if h.notify != nil && h.db != nil && req.InventoryQuantity != nil {
		var row struct {
			Qty int `gorm:"column:inventory_quantity"`
		}
		if err := h.db.WithContext(c.Request.Context()).
			Table("product_variants").
			Select("inventory_quantity").
			Where("id = ?", variantID).
			Take(&row).Error; err == nil {
			preQty = row.Qty
			preQtyKnown = true
		}
	}

	svcReq := toServiceUpdateVariantBasics(req, productID, variantID, storeID, tenantID)
	v, err := h.svc.UpdateVariantBasics(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	if preQtyKnown && v.LowStockThreshold != nil {
		threshold := *v.LowStockThreshold
		if v.InventoryQuantity <= threshold && preQty > threshold {
			h.emitLowStock(c, tenantID, storeID, v)
		}
	}

	c.JSON(http.StatusOK, ToAdminVariantResponse(v))
}

// emitLowStock fires a low_stock notification for a variant that just
// crossed its threshold. Parse failures fall through silently — the
// notification is best-effort.
func (h *VariantHandler) emitLowStock(c *gin.Context, tenantID, storeID string, v *product.Variant) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return
	}
	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		return
	}
	var variantID *uuid.UUID
	if vid, err := uuid.Parse(v.ID); err == nil {
		variantID = &vid
	}
	msg := v.SKU + " stock dropped to " + strconv.Itoa(v.InventoryQuantity) + "."
	resourceType := "variant"
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     tenantUUID,
		StoreID:      storeUUID,
		Type:         notification.TypeLowStock,
		Title:        "Low stock alert",
		Message:      &msg,
		ResourceType: &resourceType,
		ResourceID:   variantID,
	})
}
