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
