//go:build integration

package promo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These tests pin migration 000131's constraints (#726). Loosening the
// NOT NULLs on discount_type, discount_value and stripe_coupon_id was safe
// only because explicit CHECKs replaced what they guaranteed; a CHECK that is
// silently dropped or mis-written looks exactly like a CHECK that works, so
// each one is asserted here against a real Postgres.

func ptr[T any](v T) *T { return &v }

// insertPromoCode inserts a row directly, bypassing any Go-side validation, so
// the assertions are about the DATABASE and not about the model.
func insertPromoCode(db *gorm.DB, pc *promo.PromoCode) error {
	if pc.ValidFrom.IsZero() {
		pc.ValidFrom = time.Now().UTC().Add(-time.Hour)
	}
	if pc.MaxPerEmail == 0 {
		pc.MaxPerEmail = 1
	}
	if pc.CreatedBy == "" {
		pc.CreatedBy = "test"
	}
	return db.Create(pc).Error
}

// TestPromoCodes_TrialExtensionOnlyRowInserts is the point of #726: the
// console publishes codes that only extend a trial, with no discount and no
// Stripe coupon. Before 000131 this row could not exist at all.
//
// It also proves the claim 000131 relies on but does not re-state as a new
// constraint: 000060's CHECK (discount_value > 0) survives untouched and still
// admits NULL, because a CHECK passes when it evaluates to UNKNOWN.
func TestPromoCodes_TrialExtensionOnlyRowInserts(t *testing.T) {
	db := testdb.NewTx(t)

	pc := &promo.PromoCode{
		Code:               "LAUNCH50",
		TrialExtensionDays: ptr(14),
	}
	require.NoError(t, insertPromoCode(db, pc),
		"a trial-extension-only code (no discount, no coupon id) must insert")

	var got promo.PromoCode
	require.NoError(t, db.Where("id = ?", pc.ID).First(&got).Error)
	require.Nil(t, got.DiscountType, "no discount must round-trip as NULL, not as a zero value")
	require.Nil(t, got.DiscountValue)
	require.Nil(t, got.StripeCouponID)
	require.NotNil(t, got.TrialExtensionDays)
	require.Equal(t, 14, *got.TrialExtensionDays)
}

// TestPromoCodes_RowWithNoBenefitIsRejected covers promo_codes_has_benefit —
// the invariant that replaces the NOT NULLs. A code that neither discounts nor
// extends would be accepted at redemption and do nothing.
func TestPromoCodes_RowWithNoBenefitIsRejected(t *testing.T) {
	db := testdb.NewTx(t)

	err := insertPromoCode(db, &promo.PromoCode{Code: "NOTHINGATALL"})
	require.Error(t, err, "a code with neither a discount nor a trial extension must be rejected")
}

// TestPromoCodes_DiscountTypeWithoutValueIsRejected covers
// promo_codes_discount_pair. Three independently nullable columns would
// otherwise permit 'percentage' with no value.
func TestPromoCodes_DiscountTypeWithoutValueIsRejected(t *testing.T) {
	db := testdb.NewTx(t)

	err := insertPromoCode(db, &promo.PromoCode{
		Code:         "TYPENOVALUE1",
		DiscountType: ptr(promo.DiscountTypePercentage),
	})
	require.Error(t, err, "discount_type set with discount_value NULL must be rejected")
}

// TestPromoCodes_DiscountValueWithoutTypeIsRejected is the other half of the
// pair — a bare value with no type is equally meaningless.
func TestPromoCodes_DiscountValueWithoutTypeIsRejected(t *testing.T) {
	db := testdb.NewTx(t)

	err := insertPromoCode(db, &promo.PromoCode{
		Code:          "VALUENOTYPE1",
		DiscountValue: ptr(1000),
	})
	require.Error(t, err, "discount_value set with discount_type NULL must be rejected")
}

// TestPromoCodes_DiscountBearingCodeStillInserts guards against the loosening
// having broken the case that already worked.
func TestPromoCodes_DiscountBearingCodeStillInserts(t *testing.T) {
	db := testdb.NewTx(t)

	require.NoError(t, insertPromoCode(db, &promo.PromoCode{
		Code:           "ABCDEF123456",
		StripeCouponID: ptr("co_test_abcdef123456"),
		DiscountType:   ptr(promo.DiscountTypePercentage),
		DiscountValue:  ptr(1000),
	}), "a discount-bearing code must still insert unchanged")
}

// TestPromoCodes_NonPositiveDiscountValueIsStillRejected proves 000060's
// CHECK (discount_value > 0) is still enforced for a discount that IS present.
// "NULL passes" must not have become "anything passes".
func TestPromoCodes_NonPositiveDiscountValueIsStillRejected(t *testing.T) {
	db := testdb.NewTx(t)

	err := insertPromoCode(db, &promo.PromoCode{
		Code:          "ZERODISCOUNT",
		DiscountType:  ptr(promo.DiscountTypePercentage),
		DiscountValue: ptr(0),
	})
	require.Error(t, err, "a present discount_value must still be > 0")
}

// TestPromoCodes_TrialExtensionMustBePositive mirrors the console's own
// constraint.
func TestPromoCodes_TrialExtensionMustBePositive(t *testing.T) {
	db := testdb.NewTx(t)

	err := insertPromoCode(db, &promo.PromoCode{
		Code:               "ZEROTRIALDAY",
		TrialExtensionDays: ptr(0),
	})
	require.Error(t, err, "trial_extension_days must be > 0 when set")
}

// TestPromoCodes_ShortConsoleCodeIsAccepted is the reason the >= 12 floor had
// to go: the console's own worked example is 'LAUNCH50', 8 characters.
func TestPromoCodes_ShortConsoleCodeIsAccepted(t *testing.T) {
	db := testdb.NewTx(t)

	require.NoError(t, insertPromoCode(db, &promo.PromoCode{
		Code:               "XMAS",
		TrialExtensionDays: ptr(7),
	}), "a 4-character console code must be accepted")
}

// TestPromoCodes_DegenerateCodeIsStillRejected — the length floor was lowered,
// not removed. An empty or one-character code is a bug under any policy.
func TestPromoCodes_DegenerateCodeIsStillRejected(t *testing.T) {
	db := testdb.NewTx(t)

	for _, code := range []string{"", "X", "AB "} {
		err := insertPromoCode(db, &promo.PromoCode{
			Code:               code,
			TrialExtensionDays: ptr(7),
		})
		require.Errorf(t, err, "code %q must be rejected by the length floor", code)
	}
}
