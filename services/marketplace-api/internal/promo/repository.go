package promo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for promo codes and redemptions.
type Repository interface {
	// GetByCode returns the PromoCode for the given code string.
	// Returns apperrors.ErrNotFound when no row matches.
	GetByCode(ctx context.Context, db *gorm.DB, code string) (*PromoCode, error)

	// GetByID returns the PromoCode by primary key.
	GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*PromoCode, error)

	// Create inserts a new PromoCode row.
	Create(ctx context.Context, db *gorm.DB, pc *PromoCode) error

	// CountRedemptions returns the total redemption count for a promo code.
	CountRedemptions(ctx context.Context, db *gorm.DB, promoCodeID uuid.UUID) (int, error)

	// CountRedemptionsByEmail returns the redemption count for (promo_code, email).
	CountRedemptionsByEmail(ctx context.Context, db *gorm.DB, promoCodeID uuid.UUID, email string) (int, error)

	// GetRedemptionByStore returns the active redemption for (promo_code, store).
	GetRedemptionByStore(ctx context.Context, db *gorm.DB, promoCodeID, storeID uuid.UUID) (*Redemption, error)

	// CreateRedemption inserts a redemption row.
	CreateRedemption(ctx context.Context, db *gorm.DB, r *Redemption) error

	// DeleteRedemptionByStore removes the redemption for (promo_code_id, store_id).
	// Used by CancelPromo. Returns nil if no row matched (idempotent).
	DeleteRedemptionByStore(ctx context.Context, db *gorm.DB, promoCodeID, storeID uuid.UUID) error
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) GetByCode(ctx context.Context, db *gorm.DB, code string) (*PromoCode, error) {
	var pc PromoCode
	if err := db.WithContext(ctx).Where("code = ?", code).First(&pc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("promo_code")
		}
		return nil, fmt.Errorf("promo: get by code: %w", err)
	}
	return &pc, nil
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*PromoCode, error) {
	var pc PromoCode
	if err := db.WithContext(ctx).Where("id = ?", id).First(&pc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("promo_code")
		}
		return nil, fmt.Errorf("promo: get by id: %w", err)
	}
	return &pc, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, pc *PromoCode) error {
	if err := db.WithContext(ctx).Create(pc).Error; err != nil {
		return fmt.Errorf("promo: create: %w", err)
	}
	return nil
}

func (gormRepository) CountRedemptions(ctx context.Context, db *gorm.DB, promoCodeID uuid.UUID) (int, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&Redemption{}).
		Where("promo_code_id = ?", promoCodeID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("promo: count redemptions: %w", err)
	}
	return int(count), nil
}

func (gormRepository) CountRedemptionsByEmail(ctx context.Context, db *gorm.DB, promoCodeID uuid.UUID, email string) (int, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&Redemption{}).
		Where("promo_code_id = ? AND email = ?", promoCodeID, email).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("promo: count redemptions by email: %w", err)
	}
	return int(count), nil
}

func (gormRepository) GetRedemptionByStore(ctx context.Context, db *gorm.DB, promoCodeID, storeID uuid.UUID) (*Redemption, error) {
	var r Redemption
	if err := db.WithContext(ctx).
		Where("promo_code_id = ? AND store_id = ?", promoCodeID, storeID).
		First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("redemption")
		}
		return nil, fmt.Errorf("promo: get redemption by store: %w", err)
	}
	return &r, nil
}

func (gormRepository) CreateRedemption(ctx context.Context, db *gorm.DB, r *Redemption) error {
	if err := db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("promo: create redemption: %w", err)
	}
	return nil
}

func (gormRepository) DeleteRedemptionByStore(ctx context.Context, db *gorm.DB, promoCodeID, storeID uuid.UUID) error {
	if err := db.WithContext(ctx).
		Where("promo_code_id = ? AND store_id = ?", promoCodeID, storeID).
		Delete(&Redemption{}).Error; err != nil {
		return fmt.Errorf("promo: delete redemption: %w", err)
	}
	return nil
}
