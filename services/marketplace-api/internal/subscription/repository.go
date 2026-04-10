package subscription

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for store subscriptions.
type Repository interface {
	// GetByStoreID returns the subscription for a store.
	GetByStoreID(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*StoreSubscription, error)

	// Create inserts a new subscription row.
	Create(ctx context.Context, db *gorm.DB, s *StoreSubscription) error

	// Update saves all fields on a subscription.
	Update(ctx context.Context, db *gorm.DB, s *StoreSubscription) error

	// GetByStripeCustomerID returns the subscription by Stripe customer ID.
	GetByStripeCustomerID(ctx context.Context, db *gorm.DB, customerID string) (*StoreSubscription, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) GetByStoreID(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*StoreSubscription, error) {
	var s StoreSubscription
	if err := db.WithContext(ctx).Where("store_id = ?", storeID).First(&s).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("subscription")
		}
		return nil, fmt.Errorf("subscription get by store: %w", err)
	}
	return &s, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, s *StoreSubscription) error {
	if err := db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("subscription create: %w", err)
	}
	return nil
}

func (gormRepository) Update(ctx context.Context, db *gorm.DB, s *StoreSubscription) error {
	if err := db.WithContext(ctx).Save(s).Error; err != nil {
		return fmt.Errorf("subscription update: %w", err)
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
