package storefront

import (
	"testing"
)

func TestNewCouponValidateHandler(t *testing.T) {
	h := NewCouponValidateHandler(nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestValidateCouponResponse_Fields(t *testing.T) {
	r := ValidateCouponResponse{
		CouponID:     "test-id",
		Code:         "SAVE20",
		Type:         "percentage",
		FreeShipping: false,
		Title:        "Save 20%",
	}
	if r.Code != "SAVE20" {
		t.Errorf("expected code SAVE20, got %s", r.Code)
	}
	if r.FreeShipping {
		t.Error("expected FreeShipping to be false")
	}
}
