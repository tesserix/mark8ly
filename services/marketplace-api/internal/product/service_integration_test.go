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
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// buildService wires a product.Service against the given tx with a
// fake Uploader pre-registered against any keys the caller will use.
func buildService(tx *gorm.DB, uploader media.Uploader) *product.Service {
	return product.NewService(product.Config{
		DB:         tx,
		Repo:       product.NewRepository(tx),
		StoresRepo: stores.NewRepository(tx),
		OutboxRepo: outbox.NewRepository(tx),
		Uploader:   uploader,
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
	storeID, tenantID := seedStore(t, tx)
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

func TestIntegration_ProductService_Create_EmptyTitle_ReturnsValidationFailed(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx) // store currency = USD
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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
			Handle: "linen-shirt",
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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

	if err := svc.UpdateMedia(ctx, agg.Product.ID, storeID, tenantID, []product.MediaInput{
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
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
	storeID, tenantID := seedStore(t, tx)
	svc := buildService(tx, media.NewFakeUploader())
	err := svc.Delete(context.Background(), uuid.NewString(), storeID, tenantID)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
