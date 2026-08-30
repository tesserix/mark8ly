// Package storefront — checkout_lowstock.go: low-stock crossing detection
// for storefront sales. Kept as its own small file per this project's
// "many small files over few large files" convention — checkout_ext.go is
// already large.
//
// # Corrected 2026-08-30 (#230)
//
// This file used to claim "a customer purchase decrements stock via a DB
// trigger". No such trigger ever existed. `grep 'CREATE TRIGGER' migrations`
// returns eleven triggers and the only stock one is sync_variant_inventory(),
// which mirrors variant_stock into the denormalised
// product_variants.inventory_quantity and does nothing on order insert. A
// storefront sale changed no stock at all, and two customers could buy the
// same last unit.
//
// A sale now decrements variant_stock inside the ORDER TRANSACTION, via
// stockhold.Commit from checkout_stock.go. So the post-sale reading below is
// finally true — but by a different mechanism than the one this file
// originally described, which is worth stating because the old comment was
// load-bearing in the wrong direction: it explained away the absence of the
// thing it claimed existed.
//
// crossedLowStock still reformulates "did this write cross the threshold"
// purely in terms of post-sale stock plus the quantity just sold, since
// preSaleQty == postSaleQty + qty holds for a single sale.
//
// # Not wired
//
// crossedLowStock has no non-test caller. The predicate is correct and
// tested; nothing calls it, so no low-stock notification fires for a
// storefront sale today. Wiring it is out of scope for #230, whose
// acceptance was the decrement and this correction.
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
