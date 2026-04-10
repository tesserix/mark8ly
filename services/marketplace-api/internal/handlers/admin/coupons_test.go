package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/coupon"
)

func TestToAdminCouponResponse(t *testing.T) {
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	c := &coupon.Coupon{
		ID:          uuid.New(),
		TenantID:    uuid.New(),
		StoreID:     uuid.New(),
		Code:        "SAVE20",
		Title:       "Save 20%",
		Type:        coupon.CouponTypePercentage,
		Value:       decimal.NewFromInt(20),
		PerCustomer: 1,
		TargetType:  coupon.CouponTargetAll,
		StartsAt:    now,
		EndsAt:      &expires,
		Status:      coupon.CouponStatusActive,
		UsageCount:  5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	r := toAdminCouponResponse(c)

	if r.ID != c.ID.String() {
		t.Errorf("expected ID %s, got %s", c.ID, r.ID)
	}
	if r.Code != "SAVE20" {
		t.Errorf("expected code SAVE20, got %s", r.Code)
	}
	if r.Type != "percentage" {
		t.Errorf("expected type percentage, got %s", r.Type)
	}
	if r.UsageCount != 5 {
		t.Errorf("expected usage_count 5, got %d", r.UsageCount)
	}
	if r.EndsAt == nil {
		t.Error("expected EndsAt to be set")
	}
	if len(r.TargetIDs) != 0 {
		t.Errorf("expected empty TargetIDs, got %v", r.TargetIDs)
	}
}

func TestToAdminCouponResponse_NilEndsAt(t *testing.T) {
	c := &coupon.Coupon{
		ID:         uuid.New(),
		TenantID:   uuid.New(),
		StoreID:    uuid.New(),
		Code:       "FOREVER",
		Title:      "Forever",
		Type:       coupon.CouponTypePercentage,
		Value:      decimal.NewFromInt(10),
		TargetType: coupon.CouponTargetAll,
		Status:     coupon.CouponStatusActive,
	}

	r := toAdminCouponResponse(c)
	if r.EndsAt != nil {
		t.Error("expected EndsAt to be nil")
	}
}

func TestToAdminCouponUsageResponse(t *testing.T) {
	u := &coupon.CouponUsage{
		ID:             uuid.New(),
		CouponID:       uuid.New(),
		OrderID:        uuid.New(),
		CustomerEmail:  "test@example.com",
		DiscountAmount: decimal.NewFromFloat(15.50),
		CurrencyCode:   "USD",
		CreatedAt:      time.Now(),
	}

	r := toAdminCouponUsageResponse(u)
	if r.CustomerEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", r.CustomerEmail)
	}
	if !r.DiscountAmount.Equal(decimal.NewFromFloat(15.50)) {
		t.Errorf("expected discount 15.50, got %s", r.DiscountAmount)
	}
}

func TestNewCouponHandler(t *testing.T) {
	h := NewCouponHandler(nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}
