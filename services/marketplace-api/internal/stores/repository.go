// services/marketplace-api/internal/stores/repository.go
package stores

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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

	// CountActiveOrSoftDeletedRestorable counts stores for a tenant that are
	// either active (deleted_at IS NULL) or were soft-deleted within the last
	// 30 days. Both categories occupy a plan slot because a soft-deleted store
	// within the grace window can be restored. The local stores projection in
	// marketplace-api does not carry deleted_at (the authoritative record lives
	// in platform-api), so this implementation counts all rows by tenant_id as
	// a conservative upper bound. Callers use this for the downgrade preflight
	// store-count check (§4.5.1).
	CountActiveOrSoftDeletedRestorable(ctx context.Context, tenantID uuid.UUID) (int, error)

	// CountActiveOrSoftDeletedRestorableTx is the same query but runs inside
	// the caller-supplied transaction tx. Use this variant from within
	// WithAdvisoryLock so the count is consistent with the uncommitted
	// subscription mutation in the same transaction.
	CountActiveOrSoftDeletedRestorableTx(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) (int, error)
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

// CountActiveOrSoftDeletedRestorable counts stores for a tenant using r.db.
// See Repository interface doc for why this is a conservative count.
func (r *gormRepository) CountActiveOrSoftDeletedRestorable(ctx context.Context, tenantID uuid.UUID) (int, error) {
	return countStoresByTenant(ctx, r.db, tenantID)
}

// CountActiveOrSoftDeletedRestorableTx is the transaction-scoped variant.
// Orchestrator calls stores.NewRepository(tx).CountActiveOrSoftDeletedRestorableTx(...)
// to keep the count inside the advisory-lock transaction, which is cheap because
// gormRepository holds no state beyond the *gorm.DB handle.
func (r *gormRepository) CountActiveOrSoftDeletedRestorableTx(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) (int, error) {
	return countStoresByTenant(ctx, tx, tenantID)
}

// countStoresByTenant is the shared implementation for both count methods.
// The local stores projection does not carry deleted_at (that column lives in
// platform-api's authoritative table), so we count all rows for the tenant as a
// conservative upper bound for the plan-slot preflight. The 30-day retention
// window semantics are enforced at the platform-api layer on soft-delete.
func countStoresByTenant(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int, error) {
	var n int64
	err := db.WithContext(ctx).
		Table("stores").
		Where("tenant_id = ?", tenantID).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("stores: count active or restorable: %w", err)
	}
	return int(n), nil
}
