//go:build integration

package category_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func insertStoreRow(t *testing.T, tx *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	storeID = uuid.NewString()
	tenantID = uuid.NewString()
	err := tx.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now())`,
		storeID, tenantID, "cat-test-"+storeID[:8], "Test Store", "US", "USD", "UTC", "active").Error
	if err != nil {
		t.Fatalf("insert store: %v", err)
	}
	return
}

func newCategory(storeID, tenantID, slug string) *category.Category {
	return &category.Category{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		StoreID:  storeID,
		Name:     slug,
		Slug:     slug,
		Position: 0,
		IsActive: true,
	}
}

func expectCreateFails(t *testing.T, tx *gorm.DB, repo category.Repository, c *category.Category) error {
	t.Helper()
	if err := tx.Exec("SAVEPOINT sp").Error; err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	err := repo.Create(context.Background(), c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if rbErr := tx.Exec("ROLLBACK TO SAVEPOINT sp").Error; rbErr != nil {
		t.Fatalf("rollback sp: %v", rbErr)
	}
	return err
}

func TestIntegration_Category_Create_GetByIDForStore_RoundTrip(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)
	c := newCategory(storeID, tenantID, "apparel")
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByIDForStore(ctx, c.ID, storeID, tenantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "apparel" || got.Slug != "apparel" || got.StoreID != storeID || got.TenantID != tenantID {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestIntegration_Category_GetByIDForStore_CrossTenant_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)
	c := newCategory(storeID, tenantID, "apparel")
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	otherTenant := uuid.NewString()
	_, err := repo.GetByIDForStore(ctx, c.ID, storeID, otherTenant)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_Category_Create_DuplicateSlug_Returns_SlugTaken(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)
	if err := repo.Create(ctx, newCategory(storeID, tenantID, "apparel")); err != nil {
		t.Fatalf("first create: %v", err)
	}

	dup := newCategory(storeID, tenantID, "apparel")
	err := expectCreateFails(t, tx, repo, dup)
	if !errors.Is(err, apperrors.ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != apperrors.CodeSlugTaken {
		t.Fatalf("code = %s", appErr.Code)
	}
	if appErr.Details["attempted"] != "apparel" {
		t.Fatalf("attempted = %v", appErr.Details["attempted"])
	}
}

func TestIntegration_Category_SoftDelete_Then_Recreate_SameSlug_Succeeds(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)
	orig := newCategory(storeID, tenantID, "apparel")
	if err := repo.Create(ctx, orig); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := tx.Transaction(func(tx2 *gorm.DB) error {
		return repo.SoftDeleteInTx(ctx, tx2, orig.ID, storeID, tenantID)
	}); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	recreated := newCategory(storeID, tenantID, "apparel")
	if err := repo.Create(ctx, recreated); err != nil {
		t.Fatalf("recreate same slug: %v", err)
	}
}

func TestIntegration_Category_HasChildren_TrueAndFalse(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)
	parent := newCategory(storeID, tenantID, "parent")
	if err := repo.Create(ctx, parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	n, err := repo.HasChildren(ctx, parent.ID)
	if err != nil {
		t.Fatalf("has children: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	child := newCategory(storeID, tenantID, "child")
	child.ParentID = &parent.ID
	if err := repo.Create(ctx, child); err != nil {
		t.Fatalf("create child: %v", err)
	}

	n, err = repo.HasChildren(ctx, parent.ID)
	if err != nil {
		t.Fatalf("has children 2: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestIntegration_Category_HasProducts_TrueAndFalse(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)
	cat := newCategory(storeID, tenantID, "apparel")
	if err := repo.Create(ctx, cat); err != nil {
		t.Fatalf("create: %v", err)
	}

	n, err := repo.HasProducts(ctx, cat.ID)
	if err != nil {
		t.Fatalf("has products: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
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

	n, err = repo.HasProducts(ctx, cat.ID)
	if err != nil {
		t.Fatalf("has products 2: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}

	if err := tx.Exec(`UPDATE products SET deleted_at = now() WHERE id = ?`, productID).Error; err != nil {
		t.Fatalf("soft delete product: %v", err)
	}

	n, err = repo.HasProducts(ctx, cat.ID)
	if err != nil {
		t.Fatalf("has products 3: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 after soft delete, got %d", n)
	}
}

func TestIntegration_Category_ListActiveByStoreID_ExcludesInactiveAndDeleted(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID := insertStoreRow(t, tx)

	// 2 active, non-deleted.
	a1 := newCategory(storeID, tenantID, "apparel")
	a1.Position = 0
	if err := repo.Create(ctx, a1); err != nil {
		t.Fatalf("create a1: %v", err)
	}
	a2 := newCategory(storeID, tenantID, "bags")
	a2.Position = 1
	if err := repo.Create(ctx, a2); err != nil {
		t.Fatalf("create a2: %v", err)
	}

	// Inactive.
	inactive := newCategory(storeID, tenantID, "inactive")
	inactive.IsActive = false
	if err := repo.Create(ctx, inactive); err != nil {
		t.Fatalf("create inactive: %v", err)
	}

	// Soft-deleted.
	softDeleted := newCategory(storeID, tenantID, "gone")
	if err := repo.Create(ctx, softDeleted); err != nil {
		t.Fatalf("create soft: %v", err)
	}
	if err := tx.Exec("UPDATE categories SET deleted_at = now() WHERE id = ?", softDeleted.ID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	got, err := repo.ListActiveByStoreID(ctx, storeID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %+v", len(got), got)
	}
	if got[0].Slug != "apparel" || got[1].Slug != "bags" {
		t.Errorf("unexpected order: %+v", got)
	}
}

func TestIntegration_Category_UpdateInTx_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := category.NewRepository(tx)
	ctx := context.Background()

	err := tx.Transaction(func(tx2 *gorm.DB) error {
		return repo.UpdateInTx(ctx, tx2, uuid.NewString(), uuid.NewString(), uuid.NewString(),
			map[string]any{"name": "nope"})
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
