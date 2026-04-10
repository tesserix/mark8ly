package coupon

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCoupon_IsActive(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name   string
		coupon Coupon
		want   bool
	}{
		{
			name:   "active coupon within date range",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: past, EndsAt: &future},
			want:   true,
		},
		{
			name:   "active coupon no expiry",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: past, EndsAt: nil},
			want:   true,
		},
		{
			name:   "disabled coupon",
			coupon: Coupon{Status: CouponStatusDisabled, StartsAt: past, EndsAt: &future},
			want:   false,
		},
		{
			name:   "not started yet",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: future},
			want:   false,
		},
		{
			name:   "already expired",
			coupon: Coupon{Status: CouponStatusActive, StartsAt: past, EndsAt: &past},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.IsActive(now)
			if got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoupon_HasUsageCapacity(t *testing.T) {
	limit10 := 10
	tests := []struct {
		name   string
		coupon Coupon
		want   bool
	}{
		{
			name:   "unlimited",
			coupon: Coupon{UsageLimit: nil, UsageCount: 9999},
			want:   true,
		},
		{
			name:   "under limit",
			coupon: Coupon{UsageLimit: &limit10, UsageCount: 5},
			want:   true,
		},
		{
			name:   "at limit",
			coupon: Coupon{UsageLimit: &limit10, UsageCount: 10},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.HasUsageCapacity()
			if got != tt.want {
				t.Errorf("HasUsageCapacity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCoupon_CalculateDiscount(t *testing.T) {
	maxDiscount := decimal.NewFromFloat(25.00)

	tests := []struct {
		name     string
		coupon   Coupon
		subtotal decimal.Decimal
		want     decimal.Decimal
	}{
		{
			name: "percentage 20% on $100",
			coupon: Coupon{
				Type:  CouponTypePercentage,
				Value: decimal.NewFromInt(20),
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(20.00),
		},
		{
			name: "percentage 50% on $100 capped at $25",
			coupon: Coupon{
				Type:        CouponTypePercentage,
				Value:       decimal.NewFromInt(50),
				MaxDiscount: &maxDiscount,
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(25.00),
		},
		{
			name: "fixed $15 on $100",
			coupon: Coupon{
				Type:  CouponTypeFixedAmount,
				Value: decimal.NewFromFloat(15.00),
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(15.00),
		},
		{
			name: "fixed $200 on $100 — capped at subtotal",
			coupon: Coupon{
				Type:  CouponTypeFixedAmount,
				Value: decimal.NewFromFloat(200.00),
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.NewFromFloat(100.00),
		},
		{
			name: "free shipping returns zero",
			coupon: Coupon{
				Type: CouponTypeFreeShipping,
			},
			subtotal: decimal.NewFromFloat(100.00),
			want:     decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.coupon.CalculateDiscount(tt.subtotal)
			if !got.Equal(tt.want) {
				t.Errorf("CalculateDiscount() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValidateType(t *testing.T) {
	if !ValidateType("percentage") {
		t.Error("expected percentage to be valid")
	}
	if !ValidateType("fixed_amount") {
		t.Error("expected fixed_amount to be valid")
	}
	if !ValidateType("free_shipping") {
		t.Error("expected free_shipping to be valid")
	}
	if ValidateType("bogus") {
		t.Error("expected bogus to be invalid")
	}
}

func TestCoupon_TableName(t *testing.T) {
	c := Coupon{}
	if c.TableName() != "coupons" {
		t.Errorf("expected table name 'coupons', got %q", c.TableName())
	}
}

func TestCouponUsage_TableName(t *testing.T) {
	u := CouponUsage{}
	if u.TableName() != "coupon_usage" {
		t.Errorf("expected table name 'coupon_usage', got %q", u.TableName())
	}
}

// Ensure uuid fields are uuid type (compile-time check).
var _ uuid.UUID = Coupon{}.ID
var _ uuid.UUID = CouponUsage{}.CouponID
