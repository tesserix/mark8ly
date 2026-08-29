//go:build integration

package product_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ---- helpers ----

func ptrStr(s string) *string { return &s }

// seedColorSizeProduct seeds a product with Color (Red/Blue) x Size (S/M)
// and 4 variants. Returns the created aggregate.
func seedColorSizeProduct(t *testing.T, svc *product.Service, storeID, tenantID string) *product.Aggregate {
	t.Helper()
	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Tee",
		Options: []product.OptionSpec{
			{Name: "Color", Values: []product.OptionValueSpec{{Value: "Red"}, {Value: "Blue"}}},
			{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}}},
		},
		Variants: []product.VariantInput{
			{SKU: "TEE-R-S", Price: decimal.NewFromInt(10), CurrencyCode: "USD", InitialStock: 3,
				OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Red"}, {OptionName: "Size", Value: "S"}}},
			{SKU: "TEE-R-M", Price: decimal.NewFromInt(11), CurrencyCode: "USD", InitialStock: 4,
				OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Red"}, {OptionName: "Size", Value: "M"}}},
			{SKU: "TEE-B-S", Price: decimal.NewFromInt(12), CurrencyCode: "USD", InitialStock: 5,
				OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Blue"}, {OptionName: "Size", Value: "S"}}},
			{SKU: "TEE-B-M", Price: decimal.NewFromInt(13), CurrencyCode: "USD", InitialStock: 6,
				OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Blue"}, {OptionName: "Size", Value: "M"}}},
		},
	})
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return agg
}

func findVariantBySKU(agg *product.Aggregate, sku string) *product.Variant {
	for i := range agg.Variants {
		if agg.Variants[i].SKU == sku {
			return &agg.Variants[i]
		}
	}
	return nil
}

// TestIntegration_ProductService_UpdateAggregate_FullRoundTrip exercises
// gap #1: options rename + value add + variant add/update all in one PATCH.
func TestIntegration_ProductService_UpdateAggregate_FullRoundTrip(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	agg := seedColorSizeProduct(t, svc, storeID, tenantID)
	redS := findVariantBySKU(agg, "TEE-R-S")
	if redS == nil {
		t.Fatalf("seed: TEE-R-S missing")
	}

	// Act: add value "L" to Size (so 2×3=6 variants), change one existing
	// variant's price (matched by ID), include one existing by ID unchanged,
	// add two brand-new variants without IDs for the new Size=L row.
	newPrice := decimal.NewFromInt(99)
	newOptions := []product.OptionSpec{
		{Name: "Color", Values: []product.OptionValueSpec{{Value: "Red"}, {Value: "Blue"}}},
		{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}, {Value: "L"}}},
	}
	newVariants := []product.VariantInput{
		// Existing red-s, price changed, matched by tuple (no ID needed).
		{SKU: "TEE-R-S", Price: newPrice, CurrencyCode: "USD",
			OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Red"}, {OptionName: "Size", Value: "S"}}},
		// Existing red-m, unchanged fields, matched by explicit ID.
		{ID: findVariantBySKU(agg, "TEE-R-M").ID, SKU: "TEE-R-M", Price: decimal.NewFromInt(11), CurrencyCode: "USD",
			OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Red"}, {OptionName: "Size", Value: "M"}}},
		// New red-L.
		{SKU: "TEE-R-L", Price: decimal.NewFromInt(14), CurrencyCode: "USD", InitialStock: 7,
			OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Red"}, {OptionName: "Size", Value: "L"}}},
		// Existing blue-s matched by tuple.
		{SKU: "TEE-B-S", Price: decimal.NewFromInt(12), CurrencyCode: "USD",
			OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Blue"}, {OptionName: "Size", Value: "S"}}},
		// Existing blue-m matched by tuple.
		{SKU: "TEE-B-M", Price: decimal.NewFromInt(13), CurrencyCode: "USD",
			OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Blue"}, {OptionName: "Size", Value: "M"}}},
		// Brand new blue-L.
		{SKU: "TEE-B-L", Price: decimal.NewFromInt(15), CurrencyCode: "USD", InitialStock: 8,
			OptionValues: []product.OptionValueRef{{OptionName: "Color", Value: "Blue"}, {OptionName: "Size", Value: "L"}}},
	}

	got, err := svc.UpdateAggregate(ctx, product.UpdateAggregateRequest{
		ID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		Options:  &newOptions,
		Variants: &newVariants,
	})
	if err != nil {
		t.Fatalf("UpdateAggregate: %v", err)
	}

	if len(got.Variants) != 6 {
		t.Fatalf("variants = %d, want 6", len(got.Variants))
	}
	// Existing variant IDs carried over.
	if v := findVariantBySKU(got, "TEE-R-S"); v == nil || v.ID != redS.ID {
		t.Fatalf("TEE-R-S ID did not carry over: %+v (expected %s)", v, redS.ID)
	}
	if v := findVariantBySKU(got, "TEE-R-S"); v == nil || !v.Price.Equal(newPrice) {
		t.Fatalf("TEE-R-S price = %v, want %v", v.Price, newPrice)
	}
	// New variants got fresh IDs.
	if v := findVariantBySKU(got, "TEE-R-L"); v == nil || v.ID == "" {
		t.Fatalf("TEE-R-L missing: %+v", v)
	}
	if v := findVariantBySKU(got, "TEE-B-L"); v == nil || v.ID == "" {
		t.Fatalf("TEE-B-L missing: %+v", v)
	}
	// Options: Color (2 values) + Size (3 values after adding L).
	var sizeValCount int
	for _, o := range got.Options {
		if o.Name == "Size" {
			sizeValCount = len(o.Values)
		}
	}
	if sizeValCount != 3 {
		t.Fatalf("Size values = %d, want 3 (added L)", sizeValCount)
	}
	// No duplicate variant rows.
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM product_variants WHERE product_id = ? AND deleted_at IS NULL`,
		got.Product.ID).Scan(&n).Error; err != nil {
		t.Fatalf("count variants: %v", err)
	}
	if n != 6 {
		t.Fatalf("live variant row count = %d, want 6", n)
	}
	// Exactly one product.updated outbox event for this tx.
	if evN := countProductOutbox(t, tx, got.Product.ID, "product.updated"); evN != 1 {
		t.Fatalf("product.updated outbox rows = %d, want 1", evN)
	}
}

// TestIntegration_ProductService_UpdateAggregate_RemovedVariantIDsSoftDelete
// exercises gap #2: RemovedVariantIDs path soft-deletes variants.
func TestIntegration_ProductService_UpdateAggregate_RemovedVariantIDsSoftDelete(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	agg := seedColorSizeProduct(t, svc, storeID, tenantID)
	v1 := findVariantBySKU(agg, "TEE-R-S")
	v2 := findVariantBySKU(agg, "TEE-R-M")
	if v1 == nil || v2 == nil {
		t.Fatalf("seed lookup failed")
	}

	removed := []string{v1.ID, v2.ID}
	_, err := svc.UpdateAggregate(ctx, product.UpdateAggregateRequest{
		ID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		RemovedVariantIDs: &removed,
	})
	if err != nil {
		t.Fatalf("UpdateAggregate: %v", err)
	}

	// v1, v2 have deleted_at set.
	var deletedCount int64
	if err := tx.Raw(`SELECT count(*) FROM product_variants WHERE id IN (?, ?) AND deleted_at IS NOT NULL`,
		v1.ID, v2.ID).Scan(&deletedCount).Error; err != nil {
		t.Fatalf("count deleted: %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("soft-deleted count = %d, want 2", deletedCount)
	}

	// Get returns only the active variants.
	got, err := svc.Get(ctx, agg.Product.ID, storeID, tenantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Variants) != 2 {
		t.Fatalf("active variants = %d, want 2", len(got.Variants))
	}
	// variant_option_values for soft-deleted variants still exist (FK-safe).
	var linkCount int64
	if err := tx.Raw(`SELECT count(*) FROM variant_option_values WHERE variant_id IN (?, ?)`,
		v1.ID, v2.ID).Scan(&linkCount).Error; err != nil {
		t.Fatalf("count join rows: %v", err)
	}
	if linkCount == 0 {
		t.Fatalf("expected join rows preserved for soft-deleted variants, got 0")
	}
	// Single product.updated event.
	if evN := countProductOutbox(t, tx, agg.Product.ID, "product.updated"); evN != 1 {
		t.Fatalf("product.updated outbox rows = %d, want 1", evN)
	}
}

// TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected
// exercises semantics (c): removing an option value that a surviving
// variant still references should return OptionValueInUse and roll back.
//
// This is deliberately an options-ONLY PATCH. It used to also send Variants,
// and that made the assertion structurally unreachable: two variants against a
// one-value Size option is a cartesian product of 1 against 2 rows, so
// ValidateMatrix rejected the request with variant_matrix_mismatch before the
// transaction opened, and the OptionValueInUse guard was never reached. The
// test never passed once (#446). Whenever req.Variants != nil the surviving
// variants are the ones the caller just sent, so a request that both drops a
// value and keeps a variant on it can only ever fail the matrix check —
// variant_matrix_mismatch already has coverage in service_integration_test.go.
// Sending options alone is the only shape that reaches this guard: the stored
// variants survive untouched and one of them still references Size=S.
func TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	// Seed a simpler product: Size S/M with 2 variants.
	agg, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Simple",
		Options: []product.OptionSpec{
			{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}}},
		},
		Variants: []product.VariantInput{
			{SKU: "S-1", Price: decimal.NewFromInt(10), CurrencyCode: "USD", InitialStock: 1,
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}}},
			{SKU: "S-2", Price: decimal.NewFromInt(11), CurrencyCode: "USD", InitialStock: 1,
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}}},
		},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if findVariantBySKU(agg, "S-1") == nil || findVariantBySKU(agg, "S-2") == nil {
		t.Fatalf("seed lookup failed")
	}

	// Act: remove value "S" from Size and send nothing else. The stored
	// variant S-1 survives the PATCH and still references Size=S, so the
	// value cannot be deleted.
	newOptions := []product.OptionSpec{
		{Name: "Size", Values: []product.OptionValueSpec{{Value: "M"}}},
	}

	_, err = svc.UpdateAggregate(ctx, product.UpdateAggregateRequest{
		ID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		Options: &newOptions,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, apperrors.ErrOptionValueInUse) {
		t.Fatalf("expected ErrOptionValueInUse, got %v", err)
	}

	// DB unchanged: Size still has S & M.
	var valueCount int64
	if err := tx.Raw(`
		SELECT count(*) FROM product_option_values v
		JOIN product_options o ON o.id = v.option_id
		WHERE o.product_id = ?`, agg.Product.ID).Scan(&valueCount).Error; err != nil {
		t.Fatalf("count values: %v", err)
	}
	if valueCount != 2 {
		t.Fatalf("option value count = %d, want 2 (unchanged after rollback)", valueCount)
	}
}
