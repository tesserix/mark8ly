// Package coupon implements coupon CRUD, validation, and atomic usage
// tracking for the marketplace-api. Part of Marketing M1.
package coupon

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// CouponType enumerates discount types.
type CouponType string

const (
	CouponTypePercentage   CouponType = "percentage"
	CouponTypeFixedAmount  CouponType = "fixed_amount"
	CouponTypeFreeShipping CouponType = "free_shipping"
)

// CouponStatus enumerates coupon lifecycle states.
type CouponStatus string

const (
	CouponStatusActive   CouponStatus = "active"
	CouponStatusDisabled CouponStatus = "disabled"
	CouponStatusExpired  CouponStatus = "expired"
)

// CouponTargetType enumerates what a coupon targets.
type CouponTargetType string

const (
	CouponTargetAll        CouponTargetType = "all"
	CouponTargetProducts   CouponTargetType = "products"
	CouponTargetCategories CouponTargetType = "categories"
)

// Coupon is the GORM model for the coupons table.
type Coupon struct {
	ID           uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID        `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID      uuid.UUID        `gorm:"column:store_id;type:uuid;not null"`
	Code         string           `gorm:"column:code;type:varchar(50);not null"`
	Title        string           `gorm:"column:title;type:varchar(200);not null"`
	Description  *string          `gorm:"column:description;type:text"`
	Type         CouponType       `gorm:"column:type;type:varchar(20);not null"`
	Value        decimal.Decimal  `gorm:"column:value;type:numeric(12,2);not null"`
	CurrencyCode *string          `gorm:"column:currency_code;type:char(3)"`
	MinPurchase  *decimal.Decimal `gorm:"column:min_purchase;type:numeric(12,2)"`
	MaxDiscount  *decimal.Decimal `gorm:"column:max_discount;type:numeric(12,2)"`
	UsageLimit   *int             `gorm:"column:usage_limit;type:int"`
	PerCustomer  int              `gorm:"column:per_customer;type:int;not null;default:1"`
	TargetType   CouponTargetType `gorm:"column:target_type;type:varchar(20);not null;default:all"`
	TargetIDs    pq.StringArray   `gorm:"column:target_ids;type:uuid[]"`
	Stackable    bool             `gorm:"column:stackable;type:boolean;not null;default:false"`
	StartsAt     time.Time        `gorm:"column:starts_at;not null;default:now()"`
	EndsAt       *time.Time       `gorm:"column:ends_at"`
	Status       CouponStatus     `gorm:"column:status;type:varchar(20);not null;default:active"`
	UsageCount   int              `gorm:"column:usage_count;type:int;not null;default:0"`
	CreatedAt    time.Time        `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt    time.Time        `gorm:"column:updated_at;not null;default:now()"`
}

func (Coupon) TableName() string { return "coupons" }

// CouponUsage records a single use of a coupon on an order.
type CouponUsage struct {
	ID             uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	CouponID       uuid.UUID       `gorm:"column:coupon_id;type:uuid;not null"`
	OrderID        uuid.UUID       `gorm:"column:order_id;type:uuid;not null"`
	CustomerEmail  string          `gorm:"column:customer_email;type:varchar(300);not null"`
	DiscountAmount decimal.Decimal `gorm:"column:discount_amount;type:numeric(12,2);not null"`
	CurrencyCode   string          `gorm:"column:currency_code;type:char(3);not null"`
	CreatedAt      time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (CouponUsage) TableName() string { return "coupon_usage" }

// IsActive returns true if the coupon is active, started, and not expired.
func (c *Coupon) IsActive(now time.Time) bool {
	if c.Status != CouponStatusActive {
		return false
	}
	if now.Before(c.StartsAt) {
		return false
	}
	if c.EndsAt != nil && now.After(*c.EndsAt) {
		return false
	}
	return true
}

// HasUsageCapacity returns true if the coupon has not reached its total
// usage limit. Always true when usage_limit is NULL (unlimited).
func (c *Coupon) HasUsageCapacity() bool {
	if c.UsageLimit == nil {
		return true
	}
	return c.UsageCount < *c.UsageLimit
}

// CalculateDiscount computes the discount amount for a given subtotal.
// Returns decimal.Zero for free_shipping (the discount is applied as
// shipping waiver, not subtotal reduction).
func (c *Coupon) CalculateDiscount(subtotal decimal.Decimal) decimal.Decimal {
	switch c.Type {
	case CouponTypePercentage:
		discount := subtotal.Mul(c.Value).Div(decimal.NewFromInt(100))
		if c.MaxDiscount != nil && discount.GreaterThan(*c.MaxDiscount) {
			discount = *c.MaxDiscount
		}
		return discount
	case CouponTypeFixedAmount:
		if c.Value.GreaterThan(subtotal) {
			return subtotal // never discount more than subtotal
		}
		return c.Value
	case CouponTypeFreeShipping:
		return decimal.Zero
	default:
		return decimal.Zero
	}
}

// ValidateType returns true if the coupon type string is valid.
func ValidateType(t string) bool {
	switch CouponType(t) {
	case CouponTypePercentage, CouponTypeFixedAmount, CouponTypeFreeShipping:
		return true
	}
	return false
}
