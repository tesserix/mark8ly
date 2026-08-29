//go:build integration

package product_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestIntegration_FullProductGraph_RoundTrip inserts a complete slice-1
// product graph — store → category → product → 2 options → 6 option values
// → 6 variants → 12 variant_option_values → 3 media → 2 product_categories
// → variant_stock rows → and asserts every row round-trips via Preload,
// plus that the variant_stock trigger correctly maintains
// product_variants.inventory_quantity.
func TestIntegration_FullProductGraph_RoundTrip(t *testing.T) {
	tx := testdb.NewTx(t)

	// 1. Insert store projection row (normally populated by StoreMiddleware)
	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	store := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "acme-eu-" + uuid.NewString()[:8],
		Name:         "Acme EU",
		CountryCode:  "DE",
		CurrencyCode: "EUR",
		Timezone:     "Europe/Berlin",
		Status:       stores.StatusActive,
	}
	if err := tx.Create(store).Error; err != nil {
		t.Fatalf("insert store: %v", err)
	}
	vendorID := testdb.SeedVendor(t, tx, uuid.MustParse(tenantID)).String()

	// 2. Insert two categories (one root, one nested)
	rootCat := &category.Category{
		TenantID: tenantID,
		StoreID:  storeID,
		Name:     "Apparel",
		Slug:     "apparel",
		Position: 0,
		IsActive: true,
	}
	if err := tx.Create(rootCat).Error; err != nil {
		t.Fatalf("insert root category: %v", err)
	}
	shirtCat := &category.Category{
		TenantID: tenantID,
		StoreID:  storeID,
		ParentID: &rootCat.ID,
		Name:     "Shirts",
		Slug:     "shirts",
		Position: 0,
		IsActive: true,
	}
	if err := tx.Create(shirtCat).Error; err != nil {
		t.Fatalf("insert shirt category: %v", err)
	}

	// 3. Insert the product record
	prod := &product.Product{
		TenantID:          tenantID,
		StoreID:           storeID,
		VendorID:          vendorID,
		Handle:            "linen-shirt",
		Title:             "Linen Shirt",
		Status:            product.StatusDraft,
		Tags:              []string{"summer", "linen"},
		PrimaryCategoryID: &shirtCat.ID,
	}
	if err := tx.Create(prod).Error; err != nil {
		t.Fatalf("insert product: %v", err)
	}

	// 4. Insert two options (Size, Color) with 3 and 2 values respectively
	sizeOpt := &product.Option{ProductID: prod.ID, Name: "Size", Position: 0}
	colorOpt := &product.Option{ProductID: prod.ID, Name: "Color", Position: 1}
	if err := tx.Create(sizeOpt).Error; err != nil {
		t.Fatalf("insert size option: %v", err)
	}
	if err := tx.Create(colorOpt).Error; err != nil {
		t.Fatalf("insert color option: %v", err)
	}

	sizeValues := []*product.OptionValue{
		{OptionID: sizeOpt.ID, Value: "S", Position: 0},
		{OptionID: sizeOpt.ID, Value: "M", Position: 1},
		{OptionID: sizeOpt.ID, Value: "L", Position: 2},
	}
	colorValues := []*product.OptionValue{
		{OptionID: colorOpt.ID, Value: "Sand", Position: 0},
		{OptionID: colorOpt.ID, Value: "Ink", Position: 1},
	}
	for _, v := range sizeValues {
		if err := tx.Create(v).Error; err != nil {
			t.Fatalf("insert size value %q: %v", v.Value, err)
		}
	}
	for _, v := range colorValues {
		if err := tx.Create(v).Error; err != nil {
			t.Fatalf("insert color value %q: %v", v.Value, err)
		}
	}

	// 5. Insert 6 variants (3 sizes × 2 colors) with variant_option_values joins
	//    Each variant gets a SKU and a price; inventory_quantity stays at 0 here
	//    (trigger populates it from variant_stock in step 7).
	variants := make([]*product.Variant, 0, 6)
	for si, sv := range sizeValues {
		for ci, cv := range colorValues {
			v := &product.Variant{
				ProductID:       prod.ID,
				StoreID:         storeID,
				SKU:             "LIN-" + sv.Value + "-" + cv.Value,
				Price:           decimal.NewFromFloat(89.00),
				CurrencyCode:    "EUR",
				InventoryPolicy: product.InventoryPolicyDeny,
				Position:        si*2 + ci,
			}
			if err := tx.Create(v).Error; err != nil {
				t.Fatalf("insert variant %s: %v", v.SKU, err)
			}
			if err := tx.Create(&product.VariantOptionValue{VariantID: v.ID, OptionValueID: sv.ID}).Error; err != nil {
				t.Fatalf("link variant %s to size value: %v", v.SKU, err)
			}
			if err := tx.Create(&product.VariantOptionValue{VariantID: v.ID, OptionValueID: cv.ID}).Error; err != nil {
				t.Fatalf("link variant %s to color value: %v", v.SKU, err)
			}
			variants = append(variants, v)
		}
	}
	if len(variants) != 6 {
		t.Fatalf("expected 6 variants, got %d", len(variants))
	}

	// 6. Insert 3 media rows
	mediaRows := []*product.Media{
		{ProductID: prod.ID, URL: "https://cdn.test/1.jpg", StorageKey: "tenants/" + tenantID + "/products/media/abc/1.jpg", Position: 0, MediaType: product.MediaTypeImage},
		{ProductID: prod.ID, URL: "https://cdn.test/2.jpg", StorageKey: "tenants/" + tenantID + "/products/media/def/2.jpg", Position: 1, MediaType: product.MediaTypeImage},
		{ProductID: prod.ID, VariantID: &variants[0].ID, URL: "https://cdn.test/3.jpg", StorageKey: "tenants/" + tenantID + "/products/media/ghi/3.jpg", Position: 2, MediaType: product.MediaTypeImage},
	}
	for _, m := range mediaRows {
		if err := tx.Create(m).Error; err != nil {
			t.Fatalf("insert media %s: %v", m.StorageKey, err)
		}
	}

	// 7. Insert variant_stock rows → trigger should sync inventory_quantity
	defaultLocation := "00000000-0000-0000-0000-000000000001"
	for i, v := range variants {
		stock := &product.VariantStock{
			VariantID:  v.ID,
			LocationID: defaultLocation,
			Quantity:   10 + i, // 10, 11, 12, 13, 14, 15
		}
		if err := tx.Create(stock).Error; err != nil {
			t.Fatalf("insert variant_stock for %s: %v", v.SKU, err)
		}
	}

	// 8. Insert two product_categories rows
	if err := tx.Create(&product.ProductCategory{ProductID: prod.ID, CategoryID: rootCat.ID}).Error; err != nil {
		t.Fatalf("link product to root category: %v", err)
	}
	if err := tx.Create(&product.ProductCategory{ProductID: prod.ID, CategoryID: shirtCat.ID}).Error; err != nil {
		t.Fatalf("link product to shirt category: %v", err)
	}

	// 9. Round-trip: read the product back with eager loads and verify
	var readProd product.Product
	err := tx.
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Variants.OptionValueLinks").
		Preload("Media").
		First(&readProd, "id = ?", prod.ID).Error
	if err != nil {
		t.Fatalf("read product back: %v", err)
	}

	if readProd.Handle != "linen-shirt" {
		t.Errorf("handle = %q, want linen-shirt", readProd.Handle)
	}
	if len(readProd.Options) != 2 {
		t.Errorf("options len = %d, want 2", len(readProd.Options))
	}
	if len(readProd.Variants) != 6 {
		t.Errorf("variants len = %d, want 6", len(readProd.Variants))
	}
	if len(readProd.Media) != 3 {
		t.Errorf("media len = %d, want 3", len(readProd.Media))
	}
	if len(readProd.Tags) != 2 || readProd.Tags[0] != "summer" || readProd.Tags[1] != "linen" {
		t.Errorf("tags = %v, want [summer linen]", readProd.Tags)
	}

	// 10. Verify the variant_stock trigger populated inventory_quantity.
	// We inserted variants with quantities 10..15 via variant_stock; the
	// AFTER INSERT trigger on variant_stock should have updated
	// product_variants.inventory_quantity to match.
	qtyByID := map[string]int{}
	var refreshedVariants []product.Variant
	if err := tx.Where("product_id = ?", prod.ID).Find(&refreshedVariants).Error; err != nil {
		t.Fatalf("refetch variants: %v", err)
	}
	for _, v := range refreshedVariants {
		qtyByID[v.SKU] = v.InventoryQuantity
	}
	expected := map[string]int{
		"LIN-S-Sand": 10, "LIN-S-Ink": 11,
		"LIN-M-Sand": 12, "LIN-M-Ink": 13,
		"LIN-L-Sand": 14, "LIN-L-Ink": 15,
	}
	for sku, wantQty := range expected {
		if got := qtyByID[sku]; got != wantQty {
			t.Errorf("variant_stock trigger: %s inventory_quantity = %d, want %d", sku, got, wantQty)
		}
	}

	// 11. Update variant_stock → trigger should re-sync
	firstVariantID := variants[0].ID
	if err := tx.Exec(
		"UPDATE variant_stock SET quantity = ? WHERE variant_id = ? AND location_id = ?",
		99, firstVariantID, defaultLocation,
	).Error; err != nil {
		t.Fatalf("update variant_stock: %v", err)
	}
	var updatedVariant product.Variant
	if err := tx.First(&updatedVariant, "id = ?", firstVariantID).Error; err != nil {
		t.Fatalf("refetch first variant: %v", err)
	}
	if updatedVariant.InventoryQuantity != 99 {
		t.Errorf("after update trigger: inventory_quantity = %d, want 99", updatedVariant.InventoryQuantity)
	}
}

// TestIntegration_PartialUnique_SoftDelete verifies that the partial
// unique index on (store_id, handle) WHERE deleted_at IS NULL lets a
// new live row reuse a handle after the previous row is soft-deleted.
//
// Postgres aborts an entire transaction on any SQL error, so every
// expected-failure statement (the duplicate insert, the un-delete)
// MUST run inside its own SAVEPOINT. Rolling back the savepoint after
// the expected error leaves the outer tx healthy for the next step.
func TestIntegration_PartialUnique_SoftDelete(t *testing.T) {
	tx := testdb.NewTx(t)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	store := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "acme-soft-" + uuid.NewString()[:8],
		Name:         "Acme Soft",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
		Status:       stores.StatusActive,
	}
	if err := tx.Create(store).Error; err != nil {
		t.Fatalf("insert store: %v", err)
	}
	vendorID := testdb.SeedVendor(t, tx, uuid.MustParse(tenantID)).String()

	// Insert first product (real statement; stays committed inside the outer tx)
	p1 := &product.Product{
		TenantID: tenantID,
		StoreID:  storeID,
		VendorID: vendorID,
		Handle:   "silk-scarf",
		Title:    "Silk Scarf",
		Status:   product.StatusDraft,
	}
	if err := tx.Create(p1).Error; err != nil {
		t.Fatalf("insert first product: %v", err)
	}

	// Expected-failure #1: duplicate live handle → must fail with unique
	// violation. Wrap in a savepoint so the outer tx survives the error.
	if err := tx.SavePoint("before_dup_insert").Error; err != nil {
		t.Fatalf("savepoint before_dup_insert: %v", err)
	}
	p2 := &product.Product{
		TenantID: tenantID,
		StoreID:  storeID,
		VendorID: vendorID,
		Handle:   "silk-scarf",
		Title:    "Silk Scarf (v2)",
		Status:   product.StatusDraft,
	}
	dupErr := tx.Create(p2).Error
	if dupErr == nil {
		t.Fatal("expected unique violation on duplicate live handle, got nil")
	}
	if err := tx.RollbackTo("before_dup_insert").Error; err != nil {
		t.Fatalf("rollback to before_dup_insert: %v", err)
	}

	// Soft-delete the first product (real statement)
	now := time.Now()
	if err := tx.Model(&product.Product{}).Where("id = ?", p1.ID).Update("deleted_at", now).Error; err != nil {
		t.Fatalf("soft-delete first product: %v", err)
	}

	// Now inserting the duplicate should succeed — p1 is no longer "live"
	p3 := &product.Product{
		TenantID: tenantID,
		StoreID:  storeID,
		VendorID: vendorID,
		Handle:   "silk-scarf",
		Title:    "Silk Scarf (v3)",
		Status:   product.StatusDraft,
	}
	if err := tx.Create(p3).Error; err != nil {
		t.Fatalf("insert after soft-delete: %v", err)
	}

	// Expected-failure #2: un-deleting p1 while p3 is live must fail
	// (two live rows with the same handle would violate the partial
	// unique index). Savepoint again.
	if err := tx.SavePoint("before_undelete").Error; err != nil {
		t.Fatalf("savepoint before_undelete: %v", err)
	}
	undeleteErr := tx.Model(&product.Product{}).Where("id = ?", p1.ID).
		Update("deleted_at", gorm.Expr("NULL")).Error
	if undeleteErr == nil {
		t.Fatal("expected unique violation on un-delete with conflicting live row, got nil")
	}
	if err := tx.RollbackTo("before_undelete").Error; err != nil {
		t.Fatalf("rollback to before_undelete: %v", err)
	}
}
