// Package product — repository_variants.go: ApplyVariantDiffInTx and
// UpdateVariantStockInTx.
//
// Kept separate from repository.go to keep each file under ~500 lines
// and to localise the trigger-interaction contract: the only place we
// touch `variant_stock` in the whole service lives in this file.
package product

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ApplyVariantDiffInTx applies a pre-computed diff to the product's
// variants. The caller has already matched incoming rows against
// existing rows by (option name, value) tuple and produced the add/
// update/remove sets in `diff`.
//
// Semantics:
//   - Adds: INSERT new variants (store_id copied in for composite FK).
//     OptionValueLinks are cascaded from each variant row.
//   - Updates: UPDATE a stable set of columns (sku, price,
//     compare_at_price, cost_price, weight_grams, inventory_policy,
//     low_stock_threshold, position, barcode). Currency code is NEVER
//     updated — slice 1 forbids currency change (spec §14.2). Also
//     inventory_quantity is NEVER updated here — it is derived from
//     variant_stock via a trigger; see UpdateVariantStockInTx.
//   - Removes: soft-delete (set deleted_at = now()) so historical
//     references (orders, carts) still resolve.
//
// On a unique SKU violation during an add or update, the method
// returns apperrors.SKUTaken with the conflicting SKU from the first
// failing row. This matches the Create path's error-translation
// behaviour.
func (r *gormRepository) ApplyVariantDiffInTx(ctx context.Context, tx *gorm.DB, productID, storeID string, diff VariantDiff) error {
	tx = tx.WithContext(ctx)

	// Adds
	for i := range diff.Adds {
		diff.Adds[i].ProductID = productID
		diff.Adds[i].StoreID = storeID
		// Detach OptionValueLinks so GORM's cascade doesn't double-insert
		// (see repository.go CreateAggregateInTx comment).
		links := diff.Adds[i].OptionValueLinks
		diff.Adds[i].OptionValueLinks = nil
		if err := tx.Create(&diff.Adds[i]).Error; err != nil {
			diff.Adds[i].OptionValueLinks = links
			return translateUniqueViolation(err, "", diff.Adds[i].SKU)
		}
		for j := range links {
			links[j].VariantID = diff.Adds[i].ID
		}
		if len(links) > 0 {
			if err := tx.Create(&links).Error; err != nil {
				return fmt.Errorf("product: link new variant %q: %w", diff.Adds[i].SKU, err)
			}
		}
		diff.Adds[i].OptionValueLinks = links
	}

	// Updates — deliberately whitelisted so we never touch
	// currency_code or inventory_quantity.
	for _, v := range diff.Updates {
		fields := map[string]any{
			"sku":                 v.SKU,
			"barcode":             v.Barcode,
			"price":               v.Price,
			"compare_at_price":    v.CompareAtPrice,
			"cost_price":          v.CostPrice,
			"weight_grams":        v.WeightGrams,
			"inventory_policy":    v.InventoryPolicy,
			"low_stock_threshold": v.LowStockThreshold,
			"position":            v.Position,
		}
		// The explicit "deleted_at IS NULL" here is now redundant with
		// Variant's gorm.DeletedAt, which GORM appends automatically for
		// this model. Left in place deliberately — do not remove as
		// dead code (see the Removes branch below for why it matters).
		res := tx.Model(&Variant{}).
			Where("id = ? AND product_id = ? AND store_id = ? AND deleted_at IS NULL",
				v.ID, productID, storeID).
			Updates(fields)
		if res.Error != nil {
			return translateUniqueViolation(res.Error, "", v.SKU)
		}
		if res.RowsAffected == 0 {
			return apperrors.NotFound("variant")
		}
	}

	// Removes (soft delete).
	if len(diff.Removes) > 0 {
		// Redundant with GORM's automatic filter on gorm.DeletedAt, same
		// as above — but here it is also load-bearing: without it, a
		// row already soft-deleted would match and get its deleted_at
		// re-stamped to now(), overwriting the original deletion time.
		res := tx.Model(&Variant{}).
			Where("id IN ? AND product_id = ? AND store_id = ? AND deleted_at IS NULL",
				diff.Removes, productID, storeID).
			Update("deleted_at", gorm.Expr("now()"))
		if res.Error != nil {
			return fmt.Errorf("product: remove variants: %w", res.Error)
		}
	}

	return nil
}

// UpdateVariantBasicsInTx applies a whitelisted field map to one
// product_variants row. Scoped to (product, store, tenant) via a
// join subquery so cross-tenant writes are impossible. Translates
// unique-SKU violations to apperrors.SKUTaken.
func (r *gormRepository) UpdateVariantBasicsInTx(ctx context.Context, tx *gorm.DB, productID, variantID, storeID, tenantID string, fields map[string]any) error {
	tx = tx.WithContext(ctx)
	if len(fields) == 0 {
		return nil
	}
	sku, _ := fields["sku"].(string)
	// Same deliberate redundancy as ApplyVariantDiffInTx above: GORM
	// already appends "deleted_at IS NULL" for Variant's gorm.DeletedAt,
	// but the explicit predicate stays for clarity and defense in depth.
	res := tx.Model(&Variant{}).
		Where(`id = ? AND product_id = ? AND deleted_at IS NULL AND EXISTS (
				SELECT 1 FROM products p
				WHERE p.id = product_variants.product_id
				  AND p.store_id = ? AND p.tenant_id = ? AND p.deleted_at IS NULL)`,
			variantID, productID, storeID, tenantID).
		Updates(fields)
	if res.Error != nil {
		return translateUniqueViolation(res.Error, "", sku)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("variant")
	}
	return nil
}

// UpdateVariantStockInTx upserts a single variant_stock row. This is
// the ONLY place in the whole service where we mutate
// variant_stock.quantity; the sync_variant_inventory trigger propagates
// the write to product_variants.inventory_quantity.
//
// Any attempt to write directly to product_variants.inventory_quantity
// is a bug — the test suite has a reverse-direction guard that asserts
// a direct update to inventory_quantity does NOT loop back into
// variant_stock. See repository_integration_test.go case 8b.
func (r *gormRepository) UpdateVariantStockInTx(ctx context.Context, tx *gorm.DB, variantID, locationID string, quantity int) error {
	tx = tx.WithContext(ctx)

	// Upsert — insert on first touch, update on subsequent.
	row := VariantStock{
		VariantID:  variantID,
		LocationID: locationID,
		Quantity:   quantity,
		UpdatedAt:  time.Now(),
	}
	// ON CONFLICT (variant_id, location_id) DO UPDATE SET quantity = ?.
	res := tx.Exec(`
		INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (variant_id, location_id)
		DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = now()`,
		row.VariantID, row.LocationID, row.Quantity)
	if res.Error != nil {
		return fmt.Errorf("product: upsert variant stock: %w", res.Error)
	}
	return nil
}
