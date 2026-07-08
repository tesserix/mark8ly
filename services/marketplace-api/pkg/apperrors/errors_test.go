package apperrors_test

import (
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func TestError_Is_MatchesByCode(t *testing.T) {
	err := apperrors.HandleTaken("linen-shirt", "linen-shirt-2")
	if !errors.Is(err, apperrors.ErrHandleTaken) {
		t.Fatalf("expected Is(ErrHandleTaken)==true, got err=%v", err)
	}
	var ae *apperrors.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected errors.As to match *Error")
	}
	if ae.Code != apperrors.CodeHandleTaken {
		t.Fatalf("code: got %q want %q", ae.Code, apperrors.CodeHandleTaken)
	}
	if ae.Details["suggested"] != "linen-shirt-2" {
		t.Fatalf("details.suggested: got %v", ae.Details["suggested"])
	}
}

func TestError_Codes_CoverSpec(t *testing.T) {
	want := []string{
		"validation_failed", "variant_matrix_mismatch", "too_many_options",
		"too_many_variants", "currency_mismatch", "handle_taken", "sku_taken",
		"category_not_empty", "category_has_children", "target_store_invalid",
		"upload_not_found", "forbidden", "not_found",
		"payload_too_large", "unsupported_media_type", "rate_limited",
		"currency_change_forbidden", "slug_taken",
	}
	for _, code := range want {
		if !apperrors.IsKnownCode(code) {
			t.Errorf("code %q missing from apperrors package", code)
		}
	}
}

func TestCouponErrorCodes(t *testing.T) {
	couponCodes := []string{
		"coupon_not_found",
		"coupon_expired",
		"coupon_usage_limit_reached",
		"coupon_invalid",
		"coupon_min_purchase_not_met",
	}
	for _, code := range couponCodes {
		if !apperrors.IsKnownCode(code) {
			t.Errorf("expected %q to be a known code", code)
		}
	}
}

func TestCouponNotFoundConstructor(t *testing.T) {
	err := apperrors.CouponNotFound("SAVE20")
	if err.Code != apperrors.CodeCouponNotFound {
		t.Errorf("expected code %q, got %q", apperrors.CodeCouponNotFound, err.Code)
	}
	if err.Details["code"] != "SAVE20" {
		t.Errorf("expected detail code SAVE20, got %v", err.Details["code"])
	}
}

func TestCouponExpiredConstructor(t *testing.T) {
	err := apperrors.CouponExpired("SAVE20")
	if err.Code != apperrors.CodeCouponExpired {
		t.Errorf("expected code %q, got %q", apperrors.CodeCouponExpired, err.Code)
	}
}

func TestCouponUsageLimitReachedConstructor(t *testing.T) {
	err := apperrors.CouponUsageLimitReached("SAVE20", 100)
	if err.Code != apperrors.CodeCouponUsageLimitReached {
		t.Errorf("expected code %q, got %q", apperrors.CodeCouponUsageLimitReached, err.Code)
	}
	if err.Details["usage_limit"] != 100 {
		t.Errorf("expected usage_limit 100, got %v", err.Details["usage_limit"])
	}
}

func TestCouponInvalidConstructor(t *testing.T) {
	err := apperrors.CouponInvalid("coupon is not stackable")
	if err.Code != apperrors.CodeCouponInvalid {
		t.Errorf("expected code %q, got %q", apperrors.CodeCouponInvalid, err.Code)
	}
}

func TestCouponMinPurchaseNotMetConstructor(t *testing.T) {
	err := apperrors.CouponMinPurchaseNotMet("SAVE20", "50.00", "30.00")
	if err.Code != apperrors.CodeCouponMinPurchaseNotMet {
		t.Errorf("expected code %q, got %q", apperrors.CodeCouponMinPurchaseNotMet, err.Code)
	}
}

func TestRefundUnavailable(t *testing.T) {
	err := apperrors.RefundUnavailable("no captured payment")
	if !errors.Is(err, apperrors.ErrRefundUnavailable) {
		t.Fatalf("want ErrRefundUnavailable, got %v", err)
	}
	if err.Code != apperrors.CodeRefundUnavailable {
		t.Fatalf("code: got %q want %q", err.Code, apperrors.CodeRefundUnavailable)
	}
}
