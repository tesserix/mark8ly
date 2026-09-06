package promo_test

import (
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func TestBasePriceMinorFor_DevelopedTier(t *testing.T) {
	got := promo.BasePriceMinorFor(subscription.PlanStarter, subscription.PeriodMonthly,
		subscription.PriceTierDeveloped, "usd")
	if got != 1900 {
		t.Fatalf("starter monthly usd = %d, want 1900", got)
	}
}

func TestBasePriceMinorFor_CurrencyCaseAndPaddingFolded(t *testing.T) {
	// billing_currency is a char(3) column of unspecified case, so the
	// value arriving here can be "USD" or "usd " and must find the same row.
	for _, cur := range []string{"USD", " usd", "Usd "} {
		if got := promo.BasePriceMinorFor(subscription.PlanStudio, subscription.PeriodAnnual,
			subscription.PriceTierDeveloped, cur); got != 47000 {
			t.Errorf("studio annual %q = %d, want 47000", cur, got)
		}
	}
}

func TestBasePriceMinorFor_PPPTier(t *testing.T) {
	got := promo.BasePriceMinorFor(subscription.PlanStarter, subscription.PeriodMonthly,
		subscription.PriceTierPPP, "inr")
	if got != 99900 {
		t.Fatalf("starter monthly inr = %d, want 99900", got)
	}
}

// A PPP-tier row billed in a developed currency still has a price. Reading
// only the tier's own table would report "no price" for a real subscription.
func TestBasePriceMinorFor_PPPTierBilledInDevelopedCurrency(t *testing.T) {
	got := promo.BasePriceMinorFor(subscription.PlanPro, subscription.PeriodMonthly,
		subscription.PriceTierPPP, "usd")
	if got == 0 {
		t.Fatal("pro monthly usd on a ppp-tier row returned 0; the developed table was not consulted")
	}
}

func TestBasePriceMinorFor_UnpricedPlansAndMissingCurrency(t *testing.T) {
	cases := []struct {
		name     string
		plan     subscription.SubscriptionPlan
		currency string
	}{
		{"trial has no stripe price", subscription.PlanTrial, "usd"},
		{"marketplace has no stripe price", subscription.PlanMarketplace, "usd"},
		{"null billing_currency", subscription.PlanStarter, ""},
		{"currency not in the catalog", subscription.PlanStarter, "zar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := promo.BasePriceMinorFor(tc.plan, subscription.PeriodMonthly,
				subscription.PriceTierDeveloped, tc.currency); got != 0 {
				t.Fatalf("got %d, want 0", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The defect this helper exists to close.
// ---------------------------------------------------------------------------

func percentCode(t *testing.T, bps int) *promo.PromoCode {
	t.Helper()
	typ := promo.DiscountTypePercentage
	val := bps
	return &promo.PromoCode{
		Code:          "SAVEOFFER20OFF6MONTHS",
		DiscountType:  &typ,
		DiscountValue: &val,
		MaxPerEmail:   1,
		ValidFrom:     time.Now().Add(-time.Hour),
	}
}

func validateAt(t *testing.T, plan subscription.SubscriptionPlan, currency string, base int64) promo.ValidationResult {
	t.Helper()
	return promo.Validate(promo.ValidationInput{
		SubmittedCode:  "SAVEOFFER20OFF6MONTHS",
		PromoCode:      percentCode(t, 2000),
		Now:            time.Now().UTC(),
		Plan:           plan,
		Period:         subscription.PeriodMonthly,
		BasePriceMinor: base,
		Currency:       currency,
	})
}

// The bug, pinned: a zero base price makes the discounted price zero, which
// is below every floor. Every priced plan was rejected for a reason that had
// nothing to do with the code submitted.
func TestValidate_ZeroBasePriceRejectsEveryPricedPlan(t *testing.T) {
	for _, plan := range []subscription.SubscriptionPlan{
		subscription.PlanStarter, subscription.PlanStudio, subscription.PlanPro,
	} {
		got := validateAt(t, plan, "usd", 0)
		if got.Accepted {
			t.Errorf("%s: accepted with a zero base price", plan)
		}
		if got.RejectReason != promo.RejectReasonBelowFloor {
			t.Errorf("%s: reject reason = %q, want below_absolute_floor", plan, got.RejectReason)
		}
	}
}

// And the same zero waved through the one plan the floor does not cover, so
// the old constant was not merely conservative — it was wrong in both
// directions.
func TestValidate_ZeroBasePriceAcceptsTrialPlan(t *testing.T) {
	if got := validateAt(t, subscription.PlanTrial, "usd", 0); !got.Accepted {
		t.Fatalf("trial rejected: %q", got.RejectReason)
	}
}

func TestValidate_CatalogBasePriceAcceptsStarterUSD(t *testing.T) {
	base := promo.BasePriceMinorFor(subscription.PlanStarter, subscription.PeriodMonthly,
		subscription.PriceTierDeveloped, "usd")
	got := validateAt(t, subscription.PlanStarter, "usd", base)
	if !got.Accepted {
		t.Fatalf("rejected with the real base price: %q", got.RejectReason)
	}
	if got.EffectiveMinor != 1520 {
		t.Fatalf("effective = %d, want 1520 ($19.00 less 20%%)", got.EffectiveMinor)
	}
}

// The floor still bites where the policy says it should. Starter monthly INR
// is ₹999; 20% off is ₹799.20, eighty paise under the ₹800 floor. A real
// base price does not disable the check, it makes it mean what it says.
func TestValidate_CatalogBasePriceStillEnforcesTheFloor(t *testing.T) {
	base := promo.BasePriceMinorFor(subscription.PlanStarter, subscription.PeriodMonthly,
		subscription.PriceTierPPP, "inr")
	if base != 99900 {
		t.Fatalf("base = %d, want 99900", base)
	}
	got := validateAt(t, subscription.PlanStarter, "inr", base)
	if got.Accepted {
		t.Fatal("accepted below the ₹800 absolute floor")
	}
	if got.RejectReason != promo.RejectReasonBelowFloor {
		t.Fatalf("reject reason = %q, want below_absolute_floor", got.RejectReason)
	}
}
