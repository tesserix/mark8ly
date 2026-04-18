package subscription

import (
	"context"
	"fmt"

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
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error) {
	var s StoreSubscription
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		First(&s).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
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
		Save(s)
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
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("subscription")
		}
		return nil, fmt.Errorf("subscription get by stripe customer: %w", err)
	}
	return &s, nil
}
