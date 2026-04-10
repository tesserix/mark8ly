// services/marketplace-api/internal/stores/repository.go
package stores

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository is the data-access interface for the local stores projection.
// Writes happen only through StoreMiddleware's lazy pull-through (Upsert).
// Reads are scoped by (id, tenant_id) — see ErrNotFound semantics.
type Repository interface {
	GetByIDForTenant(ctx context.Context, storeID, tenantID string) (*Store, error)
	GetBySlug(ctx context.Context, slug string) (*Store, error)
	ListForTenant(ctx context.Context, tenantID string) ([]Store, error)
	Upsert(ctx context.Context, s *Store) error
	GetProductsWatermark(ctx context.Context, storeID string) (time.Time, error)
}

// WatermarkReader is the narrow read-only contract consumed by the M6
// storefront handlers for ETag/Last-Modified generation. Implemented by
// gormRepository (so callers can share the wider Repository) but kept
// separate so handler wiring can depend on just the watermark method.
type WatermarkReader interface {
	GetProductsWatermark(ctx context.Context, storeID string) (time.Time, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) GetByIDForTenant(ctx context.Context, storeID, tenantID string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", storeID, tenantID).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stores: get by id for tenant: %w", err)
	}
	return &s, nil
}

// GetBySlug returns the store projection row by its public slug. The
// slug is unique across tenants at the platform-api level; this is the
// storefront entry point from slug-based URLs. Returns ErrNotFound when
// no row exists.
func (r *gormRepository) GetBySlug(ctx context.Context, slug string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).
		Where("slug = ?", slug).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stores: get by slug: %w", err)
	}
	return &s, nil
}

// ListForTenant returns all active stores belonging to a tenant. Used by
// the admin "list my stores" endpoint so the CopyToStoreDialog can
// enumerate copy targets. Returns an empty slice (not nil) when no stores
// exist for the tenant.
func (r *gormRepository) ListForTenant(ctx context.Context, tenantID string) ([]Store, error) {
	var out []Store
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tenantID, StatusActive).
		Order("name ASC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("stores: list for tenant: %w", err)
	}
	if out == nil {
		out = []Store{}
	}
	return out, nil
}

// GetProductsWatermark reads the products watermark for a store. When
// no row exists yet (brand-new store, no product mutations published),
// it returns (time.Unix(0,0), nil) — the epoch sentinel — rather than
// ErrNotFound, so ETag generation on empty stores doesn't fail.
func (r *gormRepository) GetProductsWatermark(ctx context.Context, storeID string) (time.Time, error) {
	var w StoreWatermark
	err := r.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Unix(0, 0), nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("stores: get products watermark: %w", err)
	}
	return w.ProductsUpdatedAt, nil
}

// Upsert writes the row keyed on primary key id. On conflict it replaces
// every non-pk column and bumps synced_at. Caller is responsible for
// setting SyncedAt to time.Now() before calling.
func (r *gormRepository) Upsert(ctx context.Context, s *Store) error {
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"tenant_id", "slug", "name", "country_code",
				"currency_code", "timezone", "status", "synced_at",
			}),
		}).
		Create(s).Error; err != nil {
		return fmt.Errorf("stores: upsert: %w", err)
	}
	return nil
}
