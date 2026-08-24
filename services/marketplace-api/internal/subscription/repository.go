package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for store subscriptions.
//
// Tenant scoping: every tenant-facing lookup (GetByStoreID, Update) requires
// an explicit tenantID parameter and filters the query by it. Passing the
// wrong tenant for a given store yields NotFound — never someone else's row.
// This closes an IDOR hole where a foreign tenant's code path could request
// another tenant's store_id and receive that subscription.
type Repository interface {
	// GetByStoreID returns the subscription for (tenantID, storeID). The
	// query filters by BOTH columns; a mismatch returns apperrors.NotFound.
	GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error)

	// Create inserts a new subscription row. The caller must populate
	// TenantID and StoreID; a zero TenantID is a contract bug and is
	// rejected with apperrors.ValidationFailed.
	Create(ctx context.Context, db *gorm.DB, s *StoreSubscription) error

	// Update saves all fields on a subscription, filtered by (tenant_id,
	// store_id). If no row matches, returns apperrors.NotFound rather than
	// silently succeeding.
	Update(ctx context.Context, db *gorm.DB, s *StoreSubscription) error

	// GetByStripeCustomerID returns the subscription by Stripe customer ID.
	//
	// WEBHOOK-ONLY: Stripe webhooks arrive without a tenant context, so this
	// lookup is intentionally tenant-agnostic. DO NOT expose it through any
	// tenant-facing API — the webhook signature is the only trust boundary
	// that justifies skipping tenant scoping here.
	GetByStripeCustomerID(ctx context.Context, db *gorm.DB, customerID string) (*StoreSubscription, error)

	// SetPendingDowngrade stores the target plan/period and effective_at on the
	// subscription row. Idempotent — overwriting a prior pending downgrade is
	// intentional (merchant can change their mind; last write wins).
	// MUST be called from inside WithAdvisoryLock.
	SetPendingDowngrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID,
		targetPlan SubscriptionPlan, targetPeriod SubscriptionPeriod,
		effectiveAt time.Time, reason string) error

	// ClearPendingDowngrade unsets the trio. Idempotent.
	// MUST be called from inside WithAdvisoryLock.
	ClearPendingDowngrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) error

	// CommitDowngrade swaps plan + subscription_period AND clears pending trio.
	// Used by the cron once all preconditions pass.
	// MUST be called from inside WithAdvisoryLock.
	CommitDowngrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID,
		newPlan SubscriptionPlan, newPeriod SubscriptionPeriod) error

	// CommitUpgrade mirrors CommitDowngrade for immediate upgrades.
	// MUST be called from inside WithAdvisoryLock.
	CommitUpgrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID,
		newPlan SubscriptionPlan, newPeriod SubscriptionPeriod, reason string) error

	// FindPendingDowngradesReady returns rows whose effective_at has passed.
	// Used by DowngradeRecheckCron. Callers process each row under WithAdvisoryLock.
	FindPendingDowngradesReady(ctx context.Context, db *gorm.DB, now time.Time) ([]StoreSubscription, error)

	// CountStoresForPlanSlot counts active stores belonging to the tenant. Used by
	// the Studio→Starter downgrade-block preflight and the cron re-check (§4.5.1).
	// Caller scopes to tenant only — plan slots are tenant-wide. Soft-deleted
	// stores within the 60-day restorable grace window also count toward the slot.
	CountStoresForPlanSlot(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int, error)

	// ListAllSubscriptions returns a page of subscriptions across EVERY
	// tenant, plus the unpaginated total.
	//
	// ESTATE-WIDE, deliberately unscoped by tenant: this serves the platform
	// console's cross-tenant billing view, which is HMAC-gated on the
	// platformadmin surface and has no tenant context at all. DO NOT call it
	// from any tenant-facing handler — those must stay tenant-scoped like
	// GetByStoreID.
	ListAllSubscriptions(ctx context.Context, db *gorm.DB, f CrossTenantFilter) ([]StoreSubscription, int64, error)
}

// CrossTenantFilter narrows the estate-wide subscription listing served by
// ListAllSubscriptions. Every field is optional — this is the platform
// operator's view, not a tenant-scoped one.
type CrossTenantFilter struct {
	Status string
	Plan   string
	Page   int
	Limit  int
}

// DefaultCrossTenantPageSize applies when the caller sends no limit.
const DefaultCrossTenantPageSize = 50

// MaxCrossTenantPageSize caps a page, mirroring the ceiling applied by the
// platform-api tenant directory (see applyDirectoryFilter).
const MaxCrossTenantPageSize = 500

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error) {
	var s StoreSubscription
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("subscription")
		}
		return nil, fmt.Errorf("subscription get by store: %w", err)
	}
	return &s, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, s *StoreSubscription) error {
	if s.TenantID == uuid.Nil {
		return apperrors.ValidationFailed("tenant_id", "tenant_id is required")
	}
	if s.StoreID == uuid.Nil {
		return apperrors.ValidationFailed("store_id", "store_id is required")
	}
	if err := db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("subscription create: %w", err)
	}
	return nil
}

func (gormRepository) Update(ctx context.Context, db *gorm.DB, s *StoreSubscription) error {
	if s.TenantID == uuid.Nil {
		return apperrors.ValidationFailed("tenant_id", "tenant_id is required")
	}
	res := db.WithContext(ctx).
		Model(&StoreSubscription{}).
		Where("tenant_id = ? AND store_id = ?", s.TenantID, s.StoreID).
		Select("*").
		Updates(s)
	if res.Error != nil {
		return fmt.Errorf("subscription update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("subscription")
	}
	return nil
}

func (gormRepository) GetByStripeCustomerID(ctx context.Context, db *gorm.DB, customerID string) (*StoreSubscription, error) {
	var s StoreSubscription
	if err := db.WithContext(ctx).Where("stripe_customer_id = ?", customerID).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("subscription")
		}
		return nil, fmt.Errorf("subscription get by stripe customer: %w", err)
	}
	return &s, nil
}

func (gormRepository) SetPendingDowngrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID,
	targetPlan SubscriptionPlan, targetPeriod SubscriptionPeriod,
	effectiveAt time.Time, reason string) error {
	res := db.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET pending_downgrade_plan         = ?,
		    pending_downgrade_period       = ?,
		    pending_downgrade_effective_at = ?,
		    last_plan_change_at            = now(),
		    last_plan_change_reason        = ?,
		    updated_at                     = now()
		WHERE tenant_id = ? AND store_id = ?`,
		targetPlan, targetPeriod, effectiveAt, reason, tenantID, storeID,
	)
	if res.Error != nil {
		return fmt.Errorf("subscription: set pending downgrade: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("subscription")
	}
	return nil
}

func (gormRepository) ClearPendingDowngrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) error {
	res := db.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET pending_downgrade_plan         = NULL,
		    pending_downgrade_period       = NULL,
		    pending_downgrade_effective_at = NULL,
		    updated_at                     = now()
		WHERE tenant_id = ? AND store_id = ?`,
		tenantID, storeID,
	)
	if res.Error != nil {
		return fmt.Errorf("subscription: clear pending downgrade: %w", res.Error)
	}
	return nil
}

func (gormRepository) CommitDowngrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID,
	newPlan SubscriptionPlan, newPeriod SubscriptionPeriod) error {
	res := db.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET plan                           = ?,
		    subscription_period            = ?,
		    pending_downgrade_plan         = NULL,
		    pending_downgrade_period       = NULL,
		    pending_downgrade_effective_at = NULL,
		    last_plan_change_at            = now(),
		    last_plan_change_reason        = 'downgrade_committed',
		    updated_at                     = now()
		WHERE tenant_id = ? AND store_id = ?`,
		newPlan, newPeriod, tenantID, storeID,
	)
	if res.Error != nil {
		return fmt.Errorf("subscription: commit downgrade: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("subscription")
	}
	return nil
}

func (gormRepository) CommitUpgrade(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID,
	newPlan SubscriptionPlan, newPeriod SubscriptionPeriod, reason string) error {
	res := db.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET plan                           = ?,
		    subscription_period            = ?,
		    pending_downgrade_plan         = NULL,
		    pending_downgrade_period       = NULL,
		    pending_downgrade_effective_at = NULL,
		    last_plan_change_at            = now(),
		    last_plan_change_reason        = ?,
		    updated_at                     = now()
		WHERE tenant_id = ? AND store_id = ?`,
		newPlan, newPeriod, reason, tenantID, storeID,
	)
	if res.Error != nil {
		return fmt.Errorf("subscription: commit upgrade: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("subscription")
	}
	return nil
}

func (gormRepository) FindPendingDowngradesReady(ctx context.Context, db *gorm.DB, now time.Time) ([]StoreSubscription, error) {
	var out []StoreSubscription
	err := db.WithContext(ctx).
		Where("pending_downgrade_effective_at IS NOT NULL AND pending_downgrade_effective_at <= ?", now).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("subscription: find pending downgrades: %w", err)
	}
	return out, nil
}

func (gormRepository) CountStoresForPlanSlot(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int, error) {
	var count int64
	// Count active stores for the tenant. Soft-deleted-but-restorable rows
	// within the 60-day grace window also count toward the slot; callers can
	// extend the WHERE clause for that once the stores table is projected here.
	err := db.WithContext(ctx).
		Table("stores").
		Where("tenant_id = ?", tenantID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("subscription: count stores for plan slot: %w", err)
	}
	return int(count), nil
}

// applyCrossTenantFilter builds the WHERE clause shared by the count and the
// page query in ListAllSubscriptions, so the two can never drift apart.
func applyCrossTenantFilter(q *gorm.DB, f CrossTenantFilter) *gorm.DB {
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Plan != "" {
		q = q.Where("plan = ?", f.Plan)
	}
	return q
}

func (gormRepository) ListAllSubscriptions(ctx context.Context, db *gorm.DB, f CrossTenantFilter) ([]StoreSubscription, int64, error) {
	var total int64
	countQ := applyCrossTenantFilter(db.WithContext(ctx).Model(&StoreSubscription{}), f)
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("subscription: list all count: %w", err)
	}

	page := f.Page
	if page <= 0 {
		page = 1
	}
	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultCrossTenantPageSize
	case limit > MaxCrossTenantPageSize:
		limit = MaxCrossTenantPageSize
	}

	// Allocate before Find: a nil slice marshals to {} downstream, which
	// defeats a caller's `?? []`.
	rows := make([]StoreSubscription, 0, limit)

	pageQ := applyCrossTenantFilter(db.WithContext(ctx).Model(&StoreSubscription{}), f)
	if err := pageQ.
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("subscription: list all: %w", err)
	}

	return rows, total, nil
}
