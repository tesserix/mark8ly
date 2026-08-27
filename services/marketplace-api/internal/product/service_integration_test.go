//go:build integration

package product_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/vendor"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// buildService wires a product.Service against the given tx with a
// fake Uploader pre-registered against any keys the caller will use.
//
// VendorLookup is wired to a real vendor.Service so svc.Create defaults
// VendorID to the tenant's self-vendor (products.vendor_id is NOT NULL).
// Callers must seed that self-vendor first — seedStore and
// seedStoreWithCurrency both do this via testdb.SeedVendor.
func buildService(tx *gorm.DB, uploader media.Uploader) *product.Service {
	return product.NewService(product.Config{
		DB:           tx,
		Repo:         product.NewRepository(tx),
		StoresRepo:   stores.NewRepository(tx),
		OutboxRepo:   outbox.NewRepository(tx),
		Uploader:     uploader,
		VendorLookup: vendor.NewService(vendor.NewRepository(tx)),
	})
}

func countProductOutbox(t *testing.T, tx *gorm.DB, productID, eventType string) int64 {
	t.Helper()
	var n int64
	err := tx.Raw(`
		SELECT count(*) FROM outbox_events
		WHERE aggregate = 'product' AND aggregate_id = ? AND event_type = ?`,
		productID, eventType).Scan(&n).Error
	if err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

func TestIntegration_ProductService_Create_SimpleProduct_HappyPath(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())

	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Title:    "Linen Shirt",
		Variants: []product.VariantInput{{
			SKU:          "LINEN-1",
			Price:        decimal.NewFromFloat(19.99),
			CurrencyCode: "USD",
			InitialStock: 5,
		}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if agg.Product.Handle != "linen-shirt" {
		t.Fatalf("handle = %q", agg.Product.Handle)
	}
	if len(agg.Variants) != 1 || agg.Variants[0].SKU != "LINEN-1" {
		t.Fatalf("variants = %+v", agg.Variants)
	}
	if agg.Product.Status != product.StatusDraft {
		t.Fatalf("status = %q, want draft", agg.Product.Status)
	}
	if n := countProductOutbox(t, tx, agg.Product.ID, "product.created"); n != 1 {
		t.Fatalf("expected 1 product.created outbox row, got %d", n)
	}
	// Stock trigger should have propagated initial stock.
	var stock int
	if err := tx.Raw(`SELECT inventory_quantity FROM product_variants WHERE id = ?`, agg.Variants[0].ID).
		Scan(&stock).Error; err != nil {
		t.Fatalf("select stock: %v", err)
	}
	if stock != 5 {
		t.Fatalf("inventory_quantity = %d, want 5", stock)
	}
}

// TestIntegration_ProductService_Create_StatusActive_SetsPublishedAt
// guards the products_published_requires_active CHECK constraint.
// Without an explicit published_at on Create with status='active', the
// row insert fails at the DB layer with a 500. Repro of the prod 500
// hit on australia-store 2026-04-25.
func TestIntegration_ProductService_Create_StatusActive_SetsPublishedAt(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())

	active := product.StatusActive
	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Title:    "Live Now",
		Status:   active,
		Variants: []product.VariantInput{{
			SKU:          "LIVE-1",
			Price:        decimal.NewFromFloat(9.99),
			CurrencyCode: "USD",
			InitialStock: 1,
		}},
	})
	if err != nil {
		t.Fatalf("create active: %v", err)
	}
	if agg.Product.Status != product.StatusActive {
		t.Fatalf("status = %q, want active", agg.Product.Status)
	}
	if agg.Product.PublishedAt == nil {
		t.Fatal("PublishedAt is nil for status=active — would violate products_published_requires_active CHECK")
	}
}

func TestIntegration_ProductService_Create_EmptyTitle_ReturnsValidationFailed(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())

	_, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "  ",
		Variants: []product.VariantInput{{SKU: "X", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	})
	if !errors.Is(err, apperrors.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}
}

func TestIntegration_ProductService_Create_CurrencyMismatch(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx) // store currency = USD
	svc := buildService(tx, media.NewFakeUploader())

	_, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Eur Product",
		Variants: []product.VariantInput{{
			SKU: "EUR-1", Price: decimal.NewFromInt(10), CurrencyCode: "EUR",
		}},
	})
	if !errors.Is(err, apperrors.ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}
}

func TestIntegration_ProductService_Create_WithOptions_Matrix(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())

	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Tee",
		Options: []product.OptionSpec{
			{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}}},
		},
		Variants: []product.VariantInput{
			{
				SKU: "TEE-S", Price: decimal.NewFromInt(10), CurrencyCode: "USD",
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}},
			},
			{
				SKU: "TEE-M", Price: decimal.NewFromInt(10), CurrencyCode: "USD",
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(agg.Options) != 1 || len(agg.Options[0].Values) != 2 {
		t.Fatalf("options = %+v", agg.Options)
	}
	if len(agg.Variants) != 2 {
		t.Fatalf("variants = %d", len(agg.Variants))
	}
}

func TestIntegration_ProductService_Create_MatrixMismatch(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())

	_, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Bad",
		Options: []product.OptionSpec{
			{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}}},
		},
		Variants: []product.VariantInput{{
			SKU: "BAD-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD",
			OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}},
		}},
	})
	if !errors.Is(err, apperrors.ErrVariantMatrixMismatch) {
		t.Fatalf("expected ErrVariantMatrixMismatch, got %v", err)
	}
}

func TestIntegration_ProductService_Create_UploadNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())

	_, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "With Media",
		Media: []product.MediaInput{{StorageKey: "ghost-key", URL: "https://cdn/x.jpg"}},
		Variants: []product.VariantInput{{
			SKU: "X-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD",
		}},
	})
	if !errors.Is(err, apperrors.ErrUploadNotFound) {
		t.Fatalf("expected ErrUploadNotFound, got %v", err)
	}
}

func TestIntegration_ProductService_Create_HandleCollision_ReturnsHandleTaken(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	if _, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
		Variants: []product.VariantInput{{SKU: "L-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	}); err != nil {
		t.Fatalf("first: %v", err)
	}

	err := withSavepoint(t, tx, func() error {
		_, e := svc.Create(ctx, product.CreateRequest{
			StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
			Handle:   "linen-shirt",
			Variants: []product.VariantInput{{SKU: "L-2", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
		})
		return e
	})
	if !errors.Is(err, apperrors.ErrHandleTaken) {
		t.Fatalf("expected ErrHandleTaken, got %v", err)
	}
}

func TestIntegration_ProductService_UpdateBasics_ChangesTitleAndStatus(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	agg, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
		Variants: []product.VariantInput{{SKU: "L-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newTitle := "Premium Linen Shirt"
	active := product.StatusActive
	updated, err := svc.UpdateBasics(ctx, product.UpdateBasicsRequest{
		ID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		Title: &newTitle, Status: &active,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Product.Title != newTitle {
		t.Fatalf("title = %q", updated.Product.Title)
	}
	if updated.Product.Status != product.StatusActive {
		t.Fatalf("status = %q", updated.Product.Status)
	}
	if updated.Product.PublishedAt == nil {
		t.Fatal("expected published_at to be set when moving to active")
	}
	if n := countProductOutbox(t, tx, agg.Product.ID, "product.updated"); n != 1 {
		t.Fatalf("expected 1 product.updated outbox row, got %d", n)
	}
}

func TestIntegration_ProductService_UpdateBasics_SanitizesDescription(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	agg, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
		Variants: []product.VariantInput{{SKU: "L-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dirty := `<p>ok</p><script>alert(1)</script>`
	updated, err := svc.UpdateBasics(ctx, product.UpdateBasicsRequest{
		ID: agg.Product.ID, StoreID: storeID, TenantID: tenantID,
		Description: &dirty,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Product.Description == nil {
		t.Fatal("expected description")
	}
	if strings.Contains(*updated.Product.Description, "<script") {
		t.Fatalf("script tag leaked: %q", *updated.Product.Description)
	}
}

func TestIntegration_ProductService_UpdateBasics_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	title := "Nope"
	_, err := svc.UpdateBasics(context.Background(), product.UpdateBasicsRequest{
		ID: uuid.NewString(), StoreID: storeID, TenantID: tenantID, Title: &title,
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_ProductService_UpdateMedia_ReplacesSet(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "k-a", Size: 1, ContentType: "image/jpeg"})
	up.Register(media.Attrs{StorageKey: "k-b", Size: 1, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	ctx := context.Background()

	agg, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
		Variants: []product.VariantInput{{SKU: "L-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.ReplaceMedia(ctx, agg.Product.ID, storeID, tenantID, []product.MediaInput{
		{StorageKey: "k-a", URL: "https://cdn/a.jpg", Position: 0},
		{StorageKey: "k-b", URL: "https://cdn/b.jpg", Position: 1},
	}); err != nil {
		t.Fatalf("update media: %v", err)
	}
	got, err := product.NewRepository(tx).GetByIDForStore(ctx, agg.Product.ID, storeID, tenantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Media) != 2 {
		t.Fatalf("media count = %d, want 2", len(got.Media))
	}
	if n := countProductOutbox(t, tx, agg.Product.ID, "product.updated"); n != 1 {
		t.Fatalf("expected 1 product.updated outbox row, got %d", n)
	}
}

func TestIntegration_ProductService_UpdateCategoryLinks_Replaces(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	agg, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
		Variants: []product.VariantInput{{SKU: "L-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Zero-row delete on a fresh product is safe; test empty replace.
	if err := svc.UpdateCategoryLinks(ctx, agg.Product.ID, storeID, tenantID, nil); err != nil {
		t.Fatalf("replace empty: %v", err)
	}

	catID := seedCategory(t, tx, storeID, tenantID, "apparel-svc")
	if err := svc.UpdateCategoryLinks(ctx, agg.Product.ID, storeID, tenantID, []string{catID}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM product_categories WHERE product_id = ?`, agg.Product.ID).
		Scan(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("link count = %d, want 1", n)
	}
}

func TestIntegration_ProductService_Delete_SoftDelete_EnqueuesOutbox(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	ctx := context.Background()

	agg, err := svc.Create(ctx, product.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Title: "Linen Shirt",
		Variants: []product.VariantInput{{SKU: "L-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(ctx, agg.Product.ID, storeID, tenantID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var deletedAt *string
	if err := tx.Raw(`SELECT deleted_at::text FROM products WHERE id = ?`, agg.Product.ID).
		Scan(&deletedAt).Error; err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt == nil || *deletedAt == "" {
		t.Fatalf("expected deleted_at to be set")
	}
	if n := countProductOutbox(t, tx, agg.Product.ID, "product.deleted"); n != 1 {
		t.Fatalf("expected 1 product.deleted outbox row, got %d", n)
	}
}

func TestIntegration_ProductService_Delete_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	err := svc.Delete(context.Background(), uuid.NewString(), storeID, tenantID)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ---------------- Copy ----------------

// seedStoreWithCurrency seeds a store under the given tenant with the
// given currency. Returns the new store ID.
func seedStoreWithCurrency(t *testing.T, db *gorm.DB, tenantID, currency string) string {
	t.Helper()
	storeID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "copy-" + storeID[:8],
		Name:         "Copy Store " + currency,
		CountryCode:  "US",
		CurrencyCode: currency,
		Timezone:     "UTC",
		Status:       stores.StatusActive,
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	// Idempotent per tenant: a second store under the same tenant just
	// returns the existing self-vendor.
	testdb.SeedVendor(t, db, uuid.MustParse(tenantID))
	return storeID
}

// seedRichSourceProduct creates a source product in srcStore with 2
// options × 2 values × 4 variants, 2 media, and 1 category link.
// Returns the created aggregate. Uses the service so currency is
// validated against the seeded store.
func seedRichSourceProduct(t *testing.T, tx *gorm.DB, srcStoreID, tenantID, currency string, catID string) *product.Aggregate {
	t.Helper()
	up := media.NewFakeUploader()
	up.Register(media.Attrs{StorageKey: "src-a", Size: 1, ContentType: "image/jpeg"})
	up.Register(media.Attrs{StorageKey: "src-b", Size: 1, ContentType: "image/jpeg"})
	svc := buildService(tx, up)

	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID:  srcStoreID,
		TenantID: tenantID,
		Title:    "Source Tee",
		Options: []product.OptionSpec{
			{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "M"}}},
			{Name: "Color", Values: []product.OptionValueSpec{{Value: "Red"}, {Value: "Blue"}}},
		},
		Variants: []product.VariantInput{
			{SKU: "SRC-S-R", Price: decimal.NewFromInt(10), CurrencyCode: currency, InitialStock: 3, OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}, {OptionName: "Color", Value: "Red"}}},
			{SKU: "SRC-S-B", Price: decimal.NewFromInt(11), CurrencyCode: currency, InitialStock: 4, OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}, {OptionName: "Color", Value: "Blue"}}},
			{SKU: "SRC-M-R", Price: decimal.NewFromInt(12), CurrencyCode: currency, InitialStock: 5, OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}, {OptionName: "Color", Value: "Red"}}},
			{SKU: "SRC-M-B", Price: decimal.NewFromInt(13), CurrencyCode: currency, InitialStock: 6, OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}, {OptionName: "Color", Value: "Blue"}}},
		},
		Media: []product.MediaInput{
			{StorageKey: "src-a", URL: "https://cdn/a.jpg", Position: 0},
			{StorageKey: "src-b", URL: "https://cdn/b.jpg", Position: 1},
		},
		CategoryIDs: []string{catID},
	})
	if err != nil {
		t.Fatalf("seed source product: %v", err)
	}
	return agg
}

func TestIntegration_ProductService_Copy_CrossStore_Success(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	// Same tenant, two stores with DIFFERENT currencies.
	tenantID := uuid.NewString()
	srcStoreID := seedStoreWithCurrency(t, tx, tenantID, "EUR")
	tgtStoreID := seedStoreWithCurrency(t, tx, tenantID, "USD")

	// Seed a category in source store.
	srcCatID := seedCategory(t, tx, srcStoreID, tenantID, "apparel")

	source := seedRichSourceProduct(t, tx, srcStoreID, tenantID, "EUR", srcCatID)

	svc := buildService(tx, media.NewFakeUploader())
	got, err := svc.Copy(ctx, product.CopyRequest{
		SourceProductID: source.Product.ID,
		SourceTenantID:  tenantID,
		SourceStoreID:   srcStoreID,
		TargetStoreID:   tgtStoreID,
	})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	if got.Product.StoreID != tgtStoreID {
		t.Fatalf("store_id = %q, want %q", got.Product.StoreID, tgtStoreID)
	}
	if got.Product.Status != product.StatusDraft {
		t.Fatalf("status = %q, want draft", got.Product.Status)
	}
	if got.Product.PublishedAt != nil {
		t.Fatalf("published_at = %v, want nil", got.Product.PublishedAt)
	}
	if got.Product.CopySourceProductID == nil || *got.Product.CopySourceProductID != source.Product.ID {
		t.Fatalf("copy_source_product_id = %v, want %s", got.Product.CopySourceProductID, source.Product.ID)
	}
	if len(got.Variants) != 4 {
		t.Fatalf("variants = %d, want 4", len(got.Variants))
	}
	for i, v := range got.Variants {
		if v.CurrencyCode != "EUR" {
			t.Fatalf("variant[%d] currency = %q, want EUR (source preserved)", i, v.CurrencyCode)
		}
		if v.InventoryQuantity != 0 {
			t.Fatalf("variant[%d] inventory_quantity = %d, want 0 in target store", i, v.InventoryQuantity)
		}
	}
	// Media: rows share storage_keys with source.
	if len(got.Media) != 2 {
		t.Fatalf("media = %d, want 2", len(got.Media))
	}
	srcKeys := map[string]bool{}
	for _, m := range source.Media {
		srcKeys[m.StorageKey] = true
	}
	for _, m := range got.Media {
		if !srcKeys[m.StorageKey] {
			t.Fatalf("media storage_key %q not found in source", m.StorageKey)
		}
	}
	// Category link present in target store.
	if len(got.CategoryLinks) != 1 {
		t.Fatalf("category links = %d, want 1", len(got.CategoryLinks))
	}
	var tgtCatSlug string
	if err := tx.Raw(`SELECT slug FROM categories WHERE id = ? AND store_id = ?`,
		got.CategoryLinks[0].CategoryID, tgtStoreID).Scan(&tgtCatSlug).Error; err != nil {
		t.Fatalf("select target cat: %v", err)
	}
	if tgtCatSlug != "apparel" {
		t.Fatalf("target category slug = %q, want apparel", tgtCatSlug)
	}
	// Outbox: exactly one product.created carrying store_id + copy_source_product_id.
	var payloadJSON string
	err = tx.Raw(`
		SELECT payload::text FROM outbox_events
		WHERE aggregate = 'product' AND aggregate_id = ? AND event_type = 'product.created'`,
		got.Product.ID).Scan(&payloadJSON).Error
	if err != nil {
		t.Fatalf("select outbox: %v", err)
	}
	if !strings.Contains(payloadJSON, tgtStoreID) {
		t.Fatalf("outbox payload missing target store_id: %s", payloadJSON)
	}
	if !strings.Contains(payloadJSON, source.Product.ID) {
		t.Fatalf("outbox payload missing copy_source_product_id: %s", payloadJSON)
	}
	if n := countProductOutbox(t, tx, got.Product.ID, "product.created"); n != 1 {
		t.Fatalf("expected 1 product.created outbox row for copy, got %d", n)
	}
}

func TestIntegration_ProductService_Copy_TargetInvalid_SameStore(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	srcCatID := seedCategory(t, tx, storeID, tenantID, "same-store-cat")
	source := seedRichSourceProduct(t, tx, storeID, tenantID, "USD", srcCatID)
	svc := buildService(tx, media.NewFakeUploader())

	_, err := svc.Copy(context.Background(), product.CopyRequest{
		SourceProductID: source.Product.ID,
		SourceTenantID:  tenantID,
		SourceStoreID:   storeID,
		TargetStoreID:   storeID,
	})
	if !errors.Is(err, apperrors.ErrTargetStoreInvalid) {
		t.Fatalf("expected ErrTargetStoreInvalid, got %v", err)
	}
}

func TestIntegration_ProductService_Copy_TargetInvalid_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID, _ := seedStore(t, tx)
	srcCatID := seedCategory(t, tx, storeID, tenantID, "notfound-cat")
	source := seedRichSourceProduct(t, tx, storeID, tenantID, "USD", srcCatID)
	svc := buildService(tx, media.NewFakeUploader())

	_, err := svc.Copy(context.Background(), product.CopyRequest{
		SourceProductID: source.Product.ID,
		SourceTenantID:  tenantID,
		SourceStoreID:   storeID,
		TargetStoreID:   uuid.NewString(),
	})
	if !errors.Is(err, apperrors.ErrTargetStoreInvalid) {
		t.Fatalf("expected ErrTargetStoreInvalid, got %v", err)
	}
}

func TestIntegration_ProductService_Copy_TargetInvalid_OtherTenant(t *testing.T) {
	tx := testdb.NewTx(t)
	srcStoreID, srcTenantID, _ := seedStore(t, tx)
	srcCatID := seedCategory(t, tx, srcStoreID, srcTenantID, "other-tenant-cat")
	source := seedRichSourceProduct(t, tx, srcStoreID, srcTenantID, "USD", srcCatID)

	// Target store lives under a different tenant.
	otherTenantID := uuid.NewString()
	otherStoreID := seedStoreWithCurrency(t, tx, otherTenantID, "USD")

	svc := buildService(tx, media.NewFakeUploader())
	_, err := svc.Copy(context.Background(), product.CopyRequest{
		SourceProductID: source.Product.ID,
		SourceTenantID:  srcTenantID,
		SourceStoreID:   srcStoreID,
		TargetStoreID:   otherStoreID,
	})
	if !errors.Is(err, apperrors.ErrTargetStoreInvalid) {
		t.Fatalf("expected ErrTargetStoreInvalid, got %v", err)
	}
}

func TestIntegration_ProductService_Copy_HandleCollision_AutoSuffixed(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()
	tenantID := uuid.NewString()
	srcStoreID := seedStoreWithCurrency(t, tx, tenantID, "USD")
	tgtStoreID := seedStoreWithCurrency(t, tx, tenantID, "USD")

	srcCatID := seedCategory(t, tx, srcStoreID, tenantID, "collide-cat")
	source := seedRichSourceProduct(t, tx, srcStoreID, tenantID, "USD", srcCatID)

	// Pre-create a product in the TARGET store with the same handle.
	svc := buildService(tx, media.NewFakeUploader())
	if _, err := svc.Create(ctx, product.CreateRequest{
		StoreID:  tgtStoreID,
		TenantID: tenantID,
		Title:    "Source Tee",
		Handle:   source.Product.Handle,
		Variants: []product.VariantInput{{SKU: "PRE-1", Price: decimal.NewFromInt(1), CurrencyCode: "USD"}},
	}); err != nil {
		t.Fatalf("pre-seed in target: %v", err)
	}

	got, err := svc.Copy(ctx, product.CopyRequest{
		SourceProductID: source.Product.ID,
		SourceTenantID:  tenantID,
		SourceStoreID:   srcStoreID,
		TargetStoreID:   tgtStoreID,
	})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got.Product.Handle == source.Product.Handle {
		t.Fatalf("handle = %q, expected auto-suffixed", got.Product.Handle)
	}
	if !strings.HasPrefix(got.Product.Handle, source.Product.Handle+"-") {
		t.Fatalf("handle = %q, expected prefix %q-", got.Product.Handle, source.Product.Handle)
	}
}

func TestIntegration_ProductService_Copy_CategoriesCreatedInTarget(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()
	tenantID := uuid.NewString()
	srcStoreID := seedStoreWithCurrency(t, tx, tenantID, "USD")
	tgtStoreID := seedStoreWithCurrency(t, tx, tenantID, "USD")

	srcCatID := seedCategory(t, tx, srcStoreID, tenantID, "only-in-source")
	source := seedRichSourceProduct(t, tx, srcStoreID, tenantID, "USD", srcCatID)

	// Precondition: target does NOT have "only-in-source".
	var preCount int64
	if err := tx.Raw(`SELECT count(*) FROM categories WHERE store_id = ? AND slug = ?`,
		tgtStoreID, "only-in-source").Scan(&preCount).Error; err != nil {
		t.Fatalf("pre count: %v", err)
	}
	if preCount != 0 {
		t.Fatalf("target store unexpectedly has slug")
	}

	svc := buildService(tx, media.NewFakeUploader())
	if _, err := svc.Copy(ctx, product.CopyRequest{
		SourceProductID: source.Product.ID,
		SourceTenantID:  tenantID,
		SourceStoreID:   srcStoreID,
		TargetStoreID:   tgtStoreID,
	}); err != nil {
		t.Fatalf("copy: %v", err)
	}

	var postCount int64
	if err := tx.Raw(`SELECT count(*) FROM categories WHERE store_id = ? AND slug = ? AND deleted_at IS NULL`,
		tgtStoreID, "only-in-source").Scan(&postCount).Error; err != nil {
		t.Fatalf("post count: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("expected category created in target store, got count = %d", postCount)
	}
}
