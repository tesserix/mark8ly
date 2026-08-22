//go:build integration

package category_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedStoreForService inserts a live store row and returns its ids.
// Named distinctly from insertStoreRow in repository_integration_test.go
// to avoid cross-file collisions in the same test package.
func seedStoreForService(t *testing.T, tx *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	storeID = uuid.NewString()
	tenantID = uuid.NewString()
	err := tx.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now())`,
		storeID, tenantID, "svc-cat-"+storeID[:8], "Svc Test Store", "US", "USD", "UTC", "active").Error
	if err != nil {
		t.Fatalf("insert store: %v", err)
	}
	return
}

func newService(tx *gorm.DB) *category.Service {
	return category.NewService(category.Config{
		DB:         tx,
		Repo:       category.NewRepository(tx),
		OutboxRepo: outbox.NewRepository(tx),
	})
}

func countOutbox(t *testing.T, tx *gorm.DB, categoryID, eventType string) int64 {
	t.Helper()
	var n int64
	err := tx.Raw(`
		SELECT count(*) FROM outbox_events
		WHERE aggregate = 'category' AND aggregate_id = ? AND event_type = ?`,
		categoryID, eventType).Scan(&n).Error
	if err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

func TestIntegration_CategoryService_Create_HappyPath(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	storeID, tenantID := seedStoreForService(t, tx)

	cat, err := svc.Create(context.Background(), category.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Name:     "Apparel",
		Slug:     "apparel",
		Position: 1,
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cat.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if cat.Name != "Apparel" || cat.Slug != "apparel" {
		t.Fatalf("unexpected: %+v", cat)
	}
	if n := countOutbox(t, tx, cat.ID, "category.created"); n != 1 {
		t.Fatalf("expected 1 outbox row, got %d", n)
	}
	// Payload must carry store_id per publisher invariant.
	var payload string
	if err := tx.Raw(`
		SELECT payload::text FROM outbox_events
		WHERE aggregate_id = ? AND event_type = 'category.created'`,
		cat.ID).Scan(&payload).Error; err != nil {
		t.Fatalf("select payload: %v", err)
	}
	if !strings.Contains(payload, storeID) {
		t.Fatalf("payload missing store_id: %s", payload)
	}
}

func TestIntegration_CategoryService_Create_GeneratesSlugFromName(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	storeID, tenantID := seedStoreForService(t, tx)

	cat, err := svc.Create(context.Background(), category.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Name:     "Men's Apparel & More",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cat.Slug != "mens-apparel-more" {
		t.Fatalf("slug = %q, want %q", cat.Slug, "mens-apparel-more")
	}
}

func TestIntegration_CategoryService_Create_DuplicateSlug_ReturnsSlugTaken(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	storeID, tenantID := seedStoreForService(t, tx)

	if _, err := svc.Create(context.Background(), category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Apparel", Slug: "apparel", IsActive: true,
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Wrap the expected-failure create in a savepoint so the outer tx
	// stays usable after the unique-constraint violation.
	if err := tx.Exec("SAVEPOINT sp").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	_, err := svc.Create(context.Background(), category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Apparel2", Slug: "apparel", IsActive: true,
	})
	if rbErr := tx.Exec("ROLLBACK TO SAVEPOINT sp").Error; rbErr != nil {
		t.Fatalf("rollback sp: %v", rbErr)
	}
	if !errors.Is(err, apperrors.ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestIntegration_CategoryService_Create_EmptyName_ReturnsValidationFailed(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	storeID, tenantID := seedStoreForService(t, tx)

	_, err := svc.Create(context.Background(), category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "   ", IsActive: true,
	})
	if !errors.Is(err, apperrors.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}
}

func TestIntegration_CategoryService_Delete_HasChildren_Refused(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	ctx := context.Background()
	storeID, tenantID := seedStoreForService(t, tx)

	parent, err := svc.Create(ctx, category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Parent", Slug: "parent", IsActive: true,
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := svc.Create(ctx, category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Child", Slug: "child",
		ParentID: &parent.ID, IsActive: true,
	}); err != nil {
		t.Fatalf("create child: %v", err)
	}

	err = svc.Delete(ctx, parent.ID, storeID, tenantID)
	if !errors.Is(err, apperrors.ErrCategoryHasChildren) {
		t.Fatalf("expected ErrCategoryHasChildren, got %v", err)
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if got := appErr.Details["child_count"]; got != int64(1) {
		t.Fatalf("child_count = %v (%T), want int64(1)", got, got)
	}
}

func TestIntegration_CategoryService_Delete_HasProducts_Refused(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	ctx := context.Background()
	storeID, tenantID := seedStoreForService(t, tx)

	cat, err := svc.Create(ctx, category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Apparel", Slug: "apparel", IsActive: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	productID := uuid.NewString()
	handle := "p-" + productID[:8]
	if err := tx.Exec(`
		INSERT INTO products (id, tenant_id, store_id, handle, title, status)
		VALUES (?, ?, ?, ?, ?, 'draft')`,
		productID, tenantID, storeID, handle, "Test Product").Error; err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if err := tx.Exec(`
		INSERT INTO product_categories (product_id, category_id)
		VALUES (?, ?)`, productID, cat.ID).Error; err != nil {
		t.Fatalf("insert product_categories: %v", err)
	}

	err = svc.Delete(ctx, cat.ID, storeID, tenantID)
	if !errors.Is(err, apperrors.ErrCategoryNotEmpty) {
		t.Fatalf("expected ErrCategoryNotEmpty, got %v", err)
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if got := appErr.Details["product_count"]; got != int64(1) {
		t.Fatalf("product_count = %v (%T), want int64(1)", got, got)
	}
}

func TestIntegration_CategoryService_Delete_EmptyCategory_Succeeds_EnqueuesOutbox(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	ctx := context.Background()
	storeID, tenantID := seedStoreForService(t, tx)

	cat, err := svc.Create(ctx, category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Apparel", Slug: "apparel", IsActive: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, cat.ID, storeID, tenantID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var deletedAt *string
	if err := tx.Raw(`SELECT deleted_at::text FROM categories WHERE id = ?`, cat.ID).Scan(&deletedAt).Error; err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt == nil || *deletedAt == "" {
		t.Fatalf("expected deleted_at to be set, got %v", deletedAt)
	}
	if n := countOutbox(t, tx, cat.ID, "category.deleted"); n != 1 {
		t.Fatalf("expected 1 deleted outbox row, got %d", n)
	}
}

func TestIntegration_CategoryService_Update_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	storeID, tenantID := seedStoreForService(t, tx)

	name := "Nope"
	_, err := svc.Update(context.Background(), category.UpdateRequest{
		ID: uuid.NewString(), StoreID: storeID, TenantID: tenantID, Name: &name,
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_CategoryService_Update_ChangesName_EnqueuesOutbox(t *testing.T) {
	tx := testdb.NewTx(t)
	svc := newService(tx)
	ctx := context.Background()
	storeID, tenantID := seedStoreForService(t, tx)

	cat, err := svc.Create(ctx, category.CreateRequest{
		StoreID: storeID, TenantID: tenantID, Name: "Apparel", Slug: "apparel", IsActive: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newName := "Apparel & Accessories"
	updated, err := svc.Update(ctx, category.UpdateRequest{
		ID: cat.ID, StoreID: storeID, TenantID: tenantID, Name: &newName,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("name = %q, want %q", updated.Name, newName)
	}
	if n := countOutbox(t, tx, cat.ID, "category.updated"); n != 1 {
		t.Fatalf("expected 1 updated outbox row, got %d", n)
	}
}
