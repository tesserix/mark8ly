//go:build integration

package product_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants proves the
// #395 fix: Variant.DeletedAt is gorm.DeletedAt, so GORM applies its
// implicit `WHERE deleted_at IS NULL` filter to every query against the
// Variant model, including inside Preload("Variants"). Before the fix
// (Variant.DeletedAt was *time.Time), none of the five call sites below
// filtered soft-deleted variants out of the preload.
//
// It exercises the real soft-delete code path — ApplyVariantDiffInTx's
// Removes branch (repository_variants.go) — rather than a hand-written
// UPDATE, because a fabricated delete would not prove the production
// delete interacts correctly with the new GORM filter.
//
// Covers all five Preload("Variants") sites named in the plan:
//   - repository.go:209 GetByIDForStore   (admin detail)
//   - repository.go:281 ListAdmin         (admin list)
//   - repository.go:351 ListPublished     (storefront — customer-visible)
//   - repository.go:405 ListPublishedBySlugs (storefront — customer-visible)
//   - repository.go:447 GetPublishedByHandle  (storefront — customer-visible)
func TestIntegration_ProductRepo_Preload_FiltersSoftDeletedVariants(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	past := time.Now().Add(-1 * time.Hour)

	survivorID := uuid.NewString()
	removedID := uuid.NewString()
	removedSKU := "LEAK-REMOVED"

	agg := &product.Aggregate{
		Product: product.Product{
			ID:          uuid.NewString(),
			TenantID:    tenantID,
			StoreID:     storeID,
			VendorID:    &vendorID,
			Handle:      "leak-check",
			Title:       "Leak Check",
			Status:      product.StatusActive,
			PublishedAt: &past,
		},
		Variants: []product.Variant{
			{ID: survivorID, StoreID: storeID, SKU: "LEAK-SURVIVOR", Price: decimal.NewFromInt(10), CurrencyCode: "USD", InventoryPolicy: product.InventoryPolicyDeny},
			{ID: removedID, StoreID: storeID, SKU: removedSKU, Price: decimal.NewFromInt(20), CurrencyCode: "USD", InventoryPolicy: product.InventoryPolicyDeny},
		},
	}
	if err := repo.CreateAggregateInTx(ctx, tx, agg); err != nil {
		t.Fatalf("create: %v", err)
	}
	productID := agg.Product.ID

	// Soft-delete the second variant through the real production path —
	// ApplyVariantDiffInTx's Removes branch — not a hand-written UPDATE.
	if err := repo.ApplyVariantDiffInTx(ctx, tx, productID, storeID, product.VariantDiff{
		Removes: []string{removedID},
	}); err != nil {
		t.Fatalf("remove variant via real code path: %v", err)
	}

	assertOnlySurvivor := func(t *testing.T, label string, variants []product.Variant) {
		t.Helper()
		if len(variants) != 1 {
			t.Fatalf("%s: variants = %d, want 1: %+v", label, len(variants), variants)
		}
		if variants[0].ID != survivorID {
			t.Fatalf("%s: got variant %s, want survivor %s (soft-deleted variant leaked)", label, variants[0].ID, survivorID)
		}
	}

	t.Run("GetByIDForStore (admin detail)", func(t *testing.T) {
		got, err := repo.GetByIDForStore(ctx, productID, storeID, tenantID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		assertOnlySurvivor(t, "GetByIDForStore", got.Variants)
	})

	t.Run("ListAdmin (admin list)", func(t *testing.T) {
		rows, _, err := repo.ListAdmin(ctx, product.ListAdminQuery{
			StoreID: storeID, TenantID: tenantID, Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatalf("list admin: %v", err)
		}
		var found *product.Aggregate
		for i := range rows {
			if rows[i].Product.ID == productID {
				found = &rows[i]
			}
		}
		if found == nil {
			t.Fatalf("ListAdmin: product not found in results")
		}
		assertOnlySurvivor(t, "ListAdmin", found.Variants)
	})

	// Customer-visible pair — the reason this leak mattered: a removed
	// variant staying preloaded here means a customer could still see (and
	// potentially buy) a variant the merchant thought they had deleted.
	t.Run("ListPublished (storefront — customer-visible)", func(t *testing.T) {
		rows, err := repo.ListPublished(ctx, product.ListPublishedQuery{
			StoreID: storeID, Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatalf("list published: %v", err)
		}
		var found *product.Aggregate
		for i := range rows {
			if rows[i].Product.ID == productID {
				found = &rows[i]
			}
		}
		if found == nil {
			t.Fatalf("ListPublished: product not found in results")
		}
		assertOnlySurvivor(t, "ListPublished", found.Variants)
	})

	t.Run("ListPublishedBySlugs (storefront — customer-visible)", func(t *testing.T) {
		rows, err := repo.ListPublishedBySlugs(ctx, storeID, []string{"leak-check"})
		if err != nil {
			t.Fatalf("list published by slugs: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("ListPublishedBySlugs: rows = %d, want 1", len(rows))
		}
		assertOnlySurvivor(t, "ListPublishedBySlugs", rows[0].Variants)
	})

	t.Run("GetPublishedByHandle (storefront — customer-visible, PDP)", func(t *testing.T) {
		got, err := repo.GetPublishedByHandle(ctx, storeID, "leak-check")
		if err != nil {
			t.Fatalf("get published by handle: %v", err)
		}
		assertOnlySurvivor(t, "GetPublishedByHandle", got.Variants)
	})

	// The assertion that distinguishes "correctly filtered" from
	// "accidentally hard-deleted": the soft-deleted row must still exist
	// in the table. Without this, a fix that hard-deleted the row instead
	// of filtering it would also make every assertion above pass.
	t.Run("soft-deleted row still exists (not destroyed)", func(t *testing.T) {
		var stillThere product.Variant
		err := tx.Unscoped().Where("id = ?", removedID).First(&stillThere).Error
		if err != nil {
			t.Fatalf("Unscoped lookup of soft-deleted variant failed: %v (row was destroyed, not filtered)", err)
		}
		if !stillThere.DeletedAt.Valid {
			t.Fatalf("row %s exists but deleted_at is not set", removedID)
		}
		if stillThere.SKU != removedSKU {
			t.Fatalf("SKU = %q, want %q", stillThere.SKU, removedSKU)
		}
	})

	// Re-add path (per Task 1's audit): re-adding a variant with the same
	// SKU as a soft-deleted one must INSERT a new row, not fail against
	// the surviving soft-deleted row. variants_sku_per_store_live_unique
	// is a partial index (WHERE deleted_at IS NULL), so it must not
	// collide. If this fails, the #395 fix introduced a real defect.
	t.Run("re-add with same SKU as soft-deleted variant succeeds (partial index)", func(t *testing.T) {
		newID := uuid.NewString()
		err := repo.ApplyVariantDiffInTx(ctx, tx, productID, storeID, product.VariantDiff{
			Adds: []product.Variant{
				{ID: newID, StoreID: storeID, SKU: removedSKU, Price: decimal.NewFromInt(25), CurrencyCode: "USD", InventoryPolicy: product.InventoryPolicyDeny},
			},
		})
		if err != nil {
			t.Fatalf("BLOCKED: re-adding a variant with a previously-used SKU failed: %v — the partial unique index (variants_sku_per_store_live_unique, WHERE deleted_at IS NULL) should have let this insert succeed alongside the surviving soft-deleted row", err)
		}

		got, err := repo.GetByIDForStore(ctx, productID, storeID, tenantID)
		if err != nil {
			t.Fatalf("get after re-add: %v", err)
		}
		if len(got.Variants) != 2 {
			t.Fatalf("live variants after re-add = %d, want 2: %+v", len(got.Variants), got.Variants)
		}
		var newRow *product.Variant
		for i := range got.Variants {
			if got.Variants[i].ID == newID {
				newRow = &got.Variants[i]
			}
			if got.Variants[i].ID == removedID {
				t.Fatalf("soft-deleted variant %s reappeared as live after re-add — a revive happened instead of an insert", removedID)
			}
		}
		if newRow == nil {
			t.Fatalf("new variant %s not found among live variants", newID)
		}
		if newRow.SKU != removedSKU {
			t.Fatalf("new variant SKU = %q, want %q", newRow.SKU, removedSKU)
		}
		if newRow.ID == removedID {
			t.Fatalf("re-add reused the soft-deleted row's ID — expected a new row with a new UUID")
		}

		// Confirm both rows (old soft-deleted, new live) exist side by
		// side under the same SKU — the whole point of the partial index.
		var count int64
		if err := tx.Unscoped().Model(&product.Variant{}).
			Where("store_id = ? AND sku = ?", storeID, removedSKU).
			Count(&count).Error; err != nil {
			t.Fatalf("count rows with reused SKU: %v", err)
		}
		if count != 2 {
			t.Fatalf("rows with SKU %q = %d, want 2 (one soft-deleted, one live)", removedSKU, count)
		}
	})
}
