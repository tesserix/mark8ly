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

	// ListActiveOrSoftDeletedRestorable returns the full Store rows for a
	// tenant. The local stores projection does not carry deleted_at (that
	// column lives in platform-api's authoritative table), so this lists all
	// rows by tenant_id as a conservative set. Used by the downgrade preflight
	// to surface individual store details in the UI before the merchant commits.
	ListActiveOrSoftDeletedRestorable(ctx context.Context, tenantID uuid.UUID) ([]Store, error)

	// InFlightOrderCount returns the number of orders for a store that are
	// neither fulfilled nor cancelled (i.e. Pending or Confirmed). Used by the
	// downgrade preflight so merchants can see pending orders per store before
	// downgrading. Update the terminal set here if a new terminal status is added
	// to order.OrderStatus.
	InFlightOrderCount(ctx context.Context, storeID uuid.UUID) (int, error)

	// SuspendActiveForTenant flips every ACTIVE local store row for a
	// tenant to suspended, immediately — used by the platform console's
	// tenant-suspend endpoint (#287) so enforcement does not wait out
	// StoreMiddleware's FreshTTL. Over-enforcing (a store that was already
	// suspended stays suspended) is the safe direction here, so this is a
	// plain bulk UPDATE with no read-modify-write.
	SuspendActiveForTenant(ctx context.Context, tenantID string) error

	// MarkStaleForTenant forces every local store row for a tenant to be
	// treated as stale on the next read, by resetting synced_at to the
	// epoch. Used by the platform console's tenant-unsuspend endpoint
	// (#287) instead of eagerly flipping status back to active: this
	// projection has no column distinguishing a store suspended by the
	// tenant-level cascade from one suspended individually in
	// platform-api, so a local unsuspend cannot tell them apart. Forcing a
	// refetch is the only way to get an authoritative status without
	// risking that a distinction. See tenant_lifecycle.go for the full
	// rationale.
	MarkStaleForTenant(ctx context.Context, tenantID string) error
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

// ListActiveOrSoftDeletedRestorable returns all Store rows for a tenant.
// Soft-delete filtering lives in platform-api; we list all rows by tenant_id
// as a conservative upper bound for the preflight store-detail surface.
func (r *gormRepository) ListActiveOrSoftDeletedRestorable(ctx context.Context, tenantID uuid.UUID) ([]Store, error) {
	var ss []Store
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID.String()).
		Find(&ss).Error
	if err != nil {
		return nil, fmt.Errorf("stores: list active or restorable: %w", err)
	}
	return ss, nil
}

// InFlightOrderCount returns the number of orders for storeID that are not
// yet in a terminal state. In-flight = NOT (fulfilled OR cancelled). Update
// the terminal set here if a new terminal OrderStatus is added.
func (r *gormRepository) InFlightOrderCount(ctx context.Context, storeID uuid.UUID) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Table("orders").
		Where("store_id = ? AND status NOT IN ?", storeID.String(), []string{"fulfilled", "cancelled"}).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("stores: in-flight order count: %w", err)
	}
	return int(n), nil
}

// forceRefreshAge is how far in the past MarkStaleForTenant backdates
// synced_at. It is DELIBERATELY NOT the epoch: StoreMiddleware and
// SlugCache both fail OPEN on a refresh error, serving a cached row
// whose age is under their StaleCeil (24h) rather than 404ing every
// merchant during a platform-api outage (see StoreMiddleware's
// cacheErr == nil && cached != nil && time.Since(cached.SyncedAt) <
// cfg.StaleCeil branch, and SlugCache.Get's matching fallback). Backdating
// to the epoch would put every row past that 24h ceiling, so a failed
// refresh right after an unsuspend would 404 instead of serving the
// (still-accurate-enough) cached row — fail-closed where the design
// requires fail-open.
//
// forceRefreshAge sits just past the 5-minute FreshTTL default (so
// IsStale reports true and the very next read is forced through the
// refresh path) but stays far inside the 24h StaleCeil default (so a
// refresh that fails — platform-api down — still falls into the
// stale-serve branch instead of 404ing).
const forceRefreshAge = 10 * time.Minute

// SuspendActiveForTenant flips active -> suspended for every local store
// row belonging to tenantID. See Repository interface doc for why this is
// safe to over-apply (it only ever adds suspension, never removes it).
func (r *gormRepository) SuspendActiveForTenant(ctx context.Context, tenantID string) error {
	if err := r.db.WithContext(ctx).
		Model(&Store{}).
		Where("tenant_id = ? AND status = ?", tenantID, StatusActive).
		Updates(map[string]any{"status": StatusSuspended, "synced_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("stores: suspend active for tenant: %w", err)
	}
	return nil
}

// MarkStaleForTenant resets synced_at to the epoch for every local store
// row belonging to tenantID, so the next read is forced through the
// refresh path instead of serving a cached status. See Repository
// interface doc for why unsuspend uses this instead of an eager flip back
// to active.
func (r *gormRepository) MarkStaleForTenant(ctx context.Context, tenantID string) error {
	if err := r.db.WithContext(ctx).
		Model(&Store{}).
		Where("tenant_id = ?", tenantID).
		Update("synced_at", time.Now().Add(-forceRefreshAge)).Error; err != nil {
		return fmt.Errorf("stores: mark stale for tenant: %w", err)
	}
	return nil
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
