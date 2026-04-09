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
