// Package promo implements the subscription promo-code engine (§7).
// Stripe Coupon is the backend of record; promo_codes + promo_redemptions
// are the local audit trail and abuse-prevention layer.
package promo

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// DiscountType is the shape of the promo discount.
type DiscountType string

const (
	DiscountTypePercentage DiscountType = "percentage"
	DiscountTypeAmount     DiscountType = "amount"
)

// PromoCode is the GORM model for the promo_codes table.
// One row per Stripe Coupon we issue. The code field is the human-facing
// code (≥12 chars, visually-safe charset per §7.3).
type PromoCode struct {
	ID             uuid.UUID    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Code           string       `gorm:"column:code;type:varchar(64);not null;uniqueIndex"`
	StripeCouponID string       `gorm:"column:stripe_coupon_id;type:varchar(100);not null"`
	DiscountType   DiscountType `gorm:"column:discount_type;type:promo_discount_type;not null"`
	// DiscountValue: basis points for percentage, minor units for amount.
	DiscountValue                int            `gorm:"column:discount_value;not null"`
	MaxDurationMonths            *int           `gorm:"column:max_duration_months"`
	ValidFrom                    time.Time      `gorm:"column:valid_from;not null;default:now()"`
	ValidUntil                   *time.Time     `gorm:"column:valid_until"`
	MaxRedemptions               *int           `gorm:"column:max_redemptions"`
	MaxPerEmail                  int            `gorm:"column:max_per_email;not null;default:1"`
	MinEffectivePricePerCurrency datatypes.JSON `gorm:"column:min_effective_price_per_currency;type:jsonb"`
	AllowedPlans                 []string       `gorm:"column:allowed_plans;type:text[];serializer:json"`
	AnnualOnly                   bool           `gorm:"column:annual_only;not null;default:false"`
	CreatedBy                    string         `gorm:"column:created_by;not null"`
	CreatedAt                    time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt                    time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (PromoCode) TableName() string { return "promo_codes" }

// Redemption is the GORM model for the promo_redemptions table.
// One row per (promo_code, store) redemption. The unique index on
// (promo_code_id, email) enforces max_per_email at the DB level.
type Redemption struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	PromoCodeID    uuid.UUID `gorm:"column:promo_code_id;type:uuid;not null"`
	StoreID        uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
	SubscriptionID uuid.UUID `gorm:"column:subscription_id;type:uuid;not null"`
	Email          string    `gorm:"column:email;not null"`
	RedeemedAt     time.Time `gorm:"column:redeemed_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (Redemption) TableName() string { return "promo_redemptions" }
