// Package storefront — checkout_lowstock.go: low-stock crossing detection
// for storefront sales. Kept as its own small file per this project's
// "many small files over few large files" convention — checkout_ext.go is
// already large.
//
// A customer purchase decrements stock via a DB trigger (see
// internal/product/models.go: Variant.InventoryQuantity is
// trigger-maintained from variant_stock; do not write it directly), so this
// package can only observe the POST-sale quantity, never a directly
// snapshotted pre-sale quantity the way the admin manual variant-edit PATCH
// does (internal/handlers/admin/variants.go). crossedLowStock reformulates
// the same "did this write cross the threshold" predicate purely in terms
// of post-sale stock plus the quantity just sold, since
// preSaleQty == postSaleQty + qty always holds for a single sale.
package storefront

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/notification"
)

// crossedLowStock reports whether a sale of qty units just crossed the
// variant's low-stock threshold — i.e. the post-sale stock is at or under
// the threshold, but the pre-sale stock (current + qty) was above it.
// Returns false for a sale that started already at/under threshold (repeat
// sales while already low do not re-fire).
//
// The threshold-unset (nil) case is handled by the caller, which skips the
// variant entirely before calling this helper — keeping this signature
// plain ints avoids the caller having to unwrap a *int for a pure
// arithmetic check.
func crossedLowStock(current, qty, threshold int) bool {
	return current <= threshold && current+qty > threshold
}

// checkLowStockCrossings re-reads post-sale inventory for each variant
// purchased in this order and emits a low_stock notification for any
// variant whose sale just crossed its threshold. Called from Checkout
// after the order is already created — this is entirely best-effort: DB
// errors are logged and the affected variant is skipped, never propagated
// to the caller, and nothing here may alter the checkout response.
//
// Purchased (variant_id, quantity) pairs come from the request's line
// items (req.Items), not the persisted order, since that's what carries
// the raw *string variant id + int quantity shape. When the same variant
// appears in more than one line item, quantities are aggregated first so
// the crossing check runs once per variant with the full purchased
// quantity — never once per line, which could fire twice for one variant
// or check against an incomplete quantity.
func (h *CheckoutExtHandler) checkLowStockCrossings(ctx context.Context, tenantID, storeID uuid.UUID, items []CheckoutItemRequest) {
	if h.db == nil {
		return
	}

	qtyByVariant := make(map[uuid.UUID]int)
	for _, it := range items {
		if it.VariantID == nil {
			continue
		}
		vid, err := uuid.Parse(*it.VariantID)
		if err != nil {
			continue
		}
		qtyByVariant[vid] += it.Quantity
	}

	for variantID, qty := range qtyByVariant {
		var row struct {
			SKU       string `gorm:"column:sku"`
			Qty       int    `gorm:"column:inventory_quantity"`
			Threshold *int   `gorm:"column:low_stock_threshold"`
		}
		if err := h.db.WithContext(ctx).
			Table("product_variants").
			Select("sku, inventory_quantity, low_stock_threshold").
			Where("id = ?", variantID).
			Take(&row).Error; err != nil {
			h.logWarn("checkout_ext: low-stock re-read failed",
				"variant_id", variantID.String(), "err", err)
			continue
		}

		if row.Threshold == nil {
			continue
		}
		if !crossedLowStock(row.Qty, qty, *row.Threshold) {
			continue
		}

		msg := row.SKU + " stock dropped to " + strconv.Itoa(row.Qty) + "."
		resourceType := "variant"
		vid := variantID
		notification.Emit(ctx, h.notify, h.logger, notification.Notification{
			TenantID:     tenantID,
			StoreID:      storeID,
			Type:         notification.TypeLowStock,
			Title:        "Low stock alert",
			Message:      &msg,
			ResourceType: &resourceType,
			ResourceID:   &vid,
		})
	}
}
