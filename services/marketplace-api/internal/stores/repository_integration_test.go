//go:build integration

package stores_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newStore(tenantID string) *stores.Store {
	return &stores.Store{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Slug:         "acme-" + uuid.NewString()[:8],
		Name:         "Acme",
		CountryCode:  "DE",
		CurrencyCode: "EUR",
		Timezone:     "Europe/Berlin",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
}

func TestIntegration_Repository_Upsert_InsertThenUpdate(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantID := uuid.NewString()
	s := newStore(tenantID)
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	// Second upsert — same ID, different fields, bumped SyncedAt.
	updated := &stores.Store{
		ID:           s.ID,
		TenantID:     tenantID,
		Slug:         s.Slug, // keep slug to avoid unique collisions
		Name:         "Acme Renamed",
		CountryCode:  "FR",
		CurrencyCode: "EUR",
		Timezone:     "Europe/Paris",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now().Add(1 * time.Second),
	}
	if err := repo.Upsert(ctx, updated); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.GetByIDForTenant(ctx, s.ID, tenantID)
	if err != nil {
		t.Fatalf("get after upsert: %v", err)
	}
	if got.Name != "Acme Renamed" {
		t.Errorf("name = %q, want Acme Renamed", got.Name)
	}
	if got.CountryCode != "FR" {
		t.Errorf("country_code = %q, want FR", got.CountryCode)
	}
	if !got.SyncedAt.After(s.SyncedAt) {
		t.Errorf("synced_at not bumped: got %v, original %v", got.SyncedAt, s.SyncedAt)
	}
}

func TestIntegration_Repository_GetByIDForTenant_RightTenant(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantID := uuid.NewString()
	s := newStore(tenantID)
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.GetByIDForTenant(ctx, s.ID, tenantID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("id = %q, want %q", got.ID, s.ID)
	}
}

func TestIntegration_Repository_GetByIDForTenant_WrongTenant(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantID := uuid.NewString()
	otherTenantID := uuid.NewString()
	s := newStore(tenantID)
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	_, err := repo.GetByIDForTenant(ctx, s.ID, otherTenantID)
	if !errors.Is(err, stores.ErrNotFound) {
		t.Fatalf("wrong tenant: err = %v, want ErrNotFound", err)
	}
}

func TestIntegration_Repository_GetByIDForTenant_MissingID(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	_, err := repo.GetByIDForTenant(ctx, uuid.NewString(), uuid.NewString())
	if !errors.Is(err, stores.ErrNotFound) {
		t.Fatalf("missing id: err = %v, want ErrNotFound", err)
	}
}

func TestIntegration_Stores_GetBySlug_HappyPath(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	s := newStore(uuid.NewString())
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := repo.GetBySlug(ctx, s.Slug)
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if got.ID != s.ID || got.Slug != s.Slug {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestIntegration_Stores_GetBySlug_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	_, err := repo.GetBySlug(ctx, "nope-"+uuid.NewString()[:8])
	if !errors.Is(err, stores.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_Stores_GetProductsWatermark_NoRow_ReturnsEpoch(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	storeID := uuid.NewString()
	got, err := repo.GetProductsWatermark(ctx, storeID)
	if err != nil {
		t.Fatalf("watermark: %v", err)
	}
	if !got.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected epoch, got %v", got)
	}
}

func TestIntegration_Stores_GetProductsWatermark_ExistingRow_ReturnsStoredTimestamp(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	s := newStore(uuid.NewString())
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("upsert store: %v", err)
	}
	ts := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Hour)
	if err := tx.Exec(
		"INSERT INTO store_watermarks (store_id, products_updated_at) VALUES (?, ?)",
		s.ID, ts,
	).Error; err != nil {
		t.Fatalf("insert watermark: %v", err)
	}

	got, err := repo.GetProductsWatermark(ctx, s.ID)
	if err != nil {
		t.Fatalf("watermark: %v", err)
	}
	if !got.Equal(ts) {
		t.Fatalf("watermark = %v, want %v", got, ts)
	}
}

func TestIntegration_Repository_IsStale_Boundary(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantID := uuid.NewString()
	s := newStore(tenantID)
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	fresh, err := repo.GetByIDForTenant(ctx, s.ID, tenantID)
	if err != nil {
		t.Fatalf("get fresh: %v", err)
	}
	if stores.IsStale(fresh, 5*time.Minute) {
		t.Errorf("fresh store reported stale")
	}

	if err := tx.Exec(
		"UPDATE stores SET synced_at = now() - interval '6 minutes' WHERE id = ?",
		s.ID,
	).Error; err != nil {
		t.Fatalf("age synced_at: %v", err)
	}

	aged, err := repo.GetByIDForTenant(ctx, s.ID, tenantID)
	if err != nil {
		t.Fatalf("get aged: %v", err)
	}
	if !stores.IsStale(aged, 5*time.Minute) {
		t.Errorf("aged store not reported stale; synced_at=%v", aged.SyncedAt)
	}

	// Nil guard.
	if !stores.IsStale(nil, time.Hour) {
		t.Errorf("IsStale(nil) = false, want true")
	}
}

func TestIntegration_CountActiveOrSoftDeletedRestorable_CountsByTenant(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()

	// Insert 3 stores for tenantA and 1 for tenantB.
	for i := 0; i < 3; i++ {
		if err := repo.Upsert(ctx, newStore(tenantA.String())); err != nil {
			t.Fatalf("upsert tenantA store %d: %v", i, err)
		}
	}
	if err := repo.Upsert(ctx, newStore(tenantB.String())); err != nil {
		t.Fatalf("upsert tenantB store: %v", err)
	}

	count, err := repo.CountActiveOrSoftDeletedRestorable(ctx, tenantA)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	countB, err := repo.CountActiveOrSoftDeletedRestorable(ctx, tenantB)
	if err != nil {
		t.Fatalf("count tenantB: %v", err)
	}
	if countB != 1 {
		t.Errorf("countB = %d, want 1", countB)
	}
}

func TestIntegration_ListActiveOrSoftDeletedRestorable_ReturnsTenantRows(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()

	for i := 0; i < 2; i++ {
		if err := repo.Upsert(ctx, newStore(tenantA.String())); err != nil {
			t.Fatalf("upsert tenantA store %d: %v", i, err)
		}
	}
	if err := repo.Upsert(ctx, newStore(tenantB.String())); err != nil {
		t.Fatalf("upsert tenantB store: %v", err)
	}

	rows, err := repo.ListActiveOrSoftDeletedRestorable(ctx, tenantA)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("len = %d, want 2", len(rows))
	}
	for _, s := range rows {
		if s.TenantID != tenantA.String() {
			t.Errorf("unexpected tenant_id %q in result", s.TenantID)
		}
	}
}

func TestIntegration_InFlightOrderCount_CountsNonTerminalOrders(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	storeID := uuid.New()

	// Insert two in-flight orders (pending, confirmed) and one terminal (fulfilled).
	for _, status := range []string{"pending", "confirmed", "fulfilled"} {
		if err := tx.Exec(
			`INSERT INTO orders
				(id, tenant_id, store_id, order_number, idempotency_key,
				 customer_email, status, payment_status, fulfillment_status,
				 subtotal, shipping_total, tax_total, discount_total,
				 grand_total, refunded_amount, currency_code)
			 VALUES (gen_random_uuid(), gen_random_uuid(), ?, ?, ?,
				 'test@example.com', ?, 'pending', 'unfulfilled',
				 0, 0, 0, 0, 0, 0, 'USD')`,
			storeID, "ORD-"+status, "idem-"+status, status,
		).Error; err != nil {
			t.Fatalf("insert order %s: %v", status, err)
		}
	}

	count, err := repo.InFlightOrderCount(ctx, storeID)
	if err != nil {
		t.Fatalf("in-flight count: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (pending + confirmed)", count)
	}
}

func TestIntegration_CountActiveOrSoftDeletedRestorableTx_UsesPassedTx(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := stores.NewRepository(tx)
	ctx := context.Background()

	tenantID := uuid.New()
	if err := repo.Upsert(ctx, newStore(tenantID.String())); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// CountActiveOrSoftDeletedRestorableTx must see the uncommitted row inside tx.
	count, err := repo.CountActiveOrSoftDeletedRestorableTx(ctx, tx, tenantID)
	if err != nil {
		t.Fatalf("count tx: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}
