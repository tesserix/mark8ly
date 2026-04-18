package pricing_test

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Developed-market baseline lookups
// ---------------------------------------------------------------------------

func TestCatalog_DevelopedStarterMonthly_19USD(t *testing.T) {
	p, ok := pricing.LookupBaseline(pricing.PlanStarter, pricing.PeriodMonthly, pricing.TierDeveloped)
	require.True(t, ok)
	require.Equal(t, int64(1900), p.UnitAmountMinor)
	require.Equal(t, "usd", p.Currency)
}

func TestCatalog_DevelopedStarterAnnual_182USD(t *testing.T) {
	p, ok := pricing.LookupBaseline(pricing.PlanStarter, pricing.PeriodAnnual, pricing.TierDeveloped)
	require.True(t, ok)
	// Spec §4.1 — $182 (not $180; 20% discount rounded to $182)
	require.Equal(t, int64(18200), p.UnitAmountMinor)
	require.Equal(t, "usd", p.Currency)
}

func TestCatalog_DevelopedStudioMonthly_49USD(t *testing.T) {
	p, ok := pricing.LookupBaseline(pricing.PlanStudio, pricing.PeriodMonthly, pricing.TierDeveloped)
	require.True(t, ok)
	require.Equal(t, int64(4900), p.UnitAmountMinor)
}

func TestCatalog_DevelopedStudioAnnual_470USD(t *testing.T) {
	p, ok := pricing.LookupBaseline(pricing.PlanStudio, pricing.PeriodAnnual, pricing.TierDeveloped)
	require.True(t, ok)
	// Spec §4.1 — $470 (not $468)
	require.Equal(t, int64(47000), p.UnitAmountMinor)
}

func TestCatalog_DevelopedProMonthly_119USD(t *testing.T) {
	p, ok := pricing.LookupBaseline(pricing.PlanPro, pricing.PeriodMonthly, pricing.TierDeveloped)
	require.True(t, ok)
	require.Equal(t, int64(11900), p.UnitAmountMinor)
}

func TestCatalog_DevelopedProAnnual_1188USD(t *testing.T) {
	p, ok := pricing.LookupBaseline(pricing.PlanPro, pricing.PeriodAnnual, pricing.TierDeveloped)
	require.True(t, ok)
	require.Equal(t, int64(118800), p.UnitAmountMinor)
}

// ---------------------------------------------------------------------------
// PPP lookups
// ---------------------------------------------------------------------------

func TestCatalog_IndiaPPPStarterMonthly_999INR(t *testing.T) {
	p, ok := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "inr")
	require.True(t, ok)
	require.Equal(t, int64(99900), p.UnitAmountMinor)
}

func TestCatalog_IndiaPPPStarterAnnual_9599INR(t *testing.T) {
	p, ok := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodAnnual, "inr")
	require.True(t, ok)
	// Spec §4.1.1 — ₹9,599
	require.Equal(t, int64(959900), p.UnitAmountMinor)
}

func TestCatalog_IndiaPPPStudioMonthly_2499INR(t *testing.T) {
	p, ok := pricing.LookupPPPOption(pricing.PlanStudio, pricing.PeriodMonthly, "inr")
	require.True(t, ok)
	// Spec §4.1.1 — ₹2,499
	require.Equal(t, int64(249900), p.UnitAmountMinor)
}

func TestCatalog_IndiaPPPProAnnual_65999INR(t *testing.T) {
	// Pro IS included in PPP per spec §4.1.1
	p, ok := pricing.LookupPPPOption(pricing.PlanPro, pricing.PeriodAnnual, "inr")
	require.True(t, ok)
	// Spec §4.1.1 — ₹65,999/yr
	require.Equal(t, int64(6599900), p.UnitAmountMinor)
}

func TestCatalog_IDRStarterMonthly_ZeroDecimal(t *testing.T) {
	p, ok := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "idr")
	require.True(t, ok)
	// Spec §4.1.1 — Rp199,000. IDR is zero-decimal but Stripe stores ×100.
	require.Equal(t, int64(19900000), p.UnitAmountMinor, "IDR zero-decimal must be multiplied ×100 for Stripe")
}

func TestCatalog_VNDStarterMonthly_ZeroDecimal(t *testing.T) {
	p, ok := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "vnd")
	require.True(t, ok)
	// Spec §4.1.1 — ₫329,000. VND is zero-decimal; ×100 for Stripe.
	require.Equal(t, int64(32900000), p.UnitAmountMinor, "VND zero-decimal must be multiplied ×100 for Stripe")
}

// ---------------------------------------------------------------------------
// Currency options consistency
// ---------------------------------------------------------------------------

func TestCatalog_CurrencyOptionsConsistency(t *testing.T) {
	opts, ok := pricing.DevelopedCurrencyOptions(pricing.PlanStudio, pricing.PeriodAnnual)
	require.True(t, ok)
	expected := []string{"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd"}
	for _, c := range expected {
		_, present := opts[c]
		require.True(t, present, "developed Price must carry currency_options for %s", c)
	}
}

func TestCatalog_AllDevelopedPlansHave7Currencies(t *testing.T) {
	plans := []pricing.Plan{pricing.PlanStarter, pricing.PlanStudio, pricing.PlanPro}
	periods := []pricing.Period{pricing.PeriodMonthly, pricing.PeriodAnnual}
	expected := []string{"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd"}
	for _, plan := range plans {
		for _, period := range periods {
			opts, ok := pricing.DevelopedCurrencyOptions(plan, period)
			require.True(t, ok, "missing developed options for %s/%s", plan, period)
			for _, c := range expected {
				_, present := opts[c]
				require.True(t, present, "%s/%s developed options missing currency %s", plan, period, c)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// AU GST exclusive tax behavior
// ---------------------------------------------------------------------------

func TestCatalog_AUTaxBehaviorExclusive(t *testing.T) {
	opts, ok := pricing.DevelopedCurrencyOptions(pricing.PlanStarter, pricing.PeriodMonthly)
	require.True(t, ok)
	au := opts["aud"]
	require.Equal(t, "exclusive", au.TaxBehavior, "AU must have tax_behavior=exclusive per §19.4")
}

func TestCatalog_AUTaxBehaviorExclusiveAllPlans(t *testing.T) {
	plans := []pricing.Plan{pricing.PlanStarter, pricing.PlanStudio, pricing.PlanPro}
	periods := []pricing.Period{pricing.PeriodMonthly, pricing.PeriodAnnual}
	for _, plan := range plans {
		for _, period := range periods {
			opts, ok := pricing.DevelopedCurrencyOptions(plan, period)
			require.True(t, ok)
			au := opts["aud"]
			require.Equal(t, "exclusive", au.TaxBehavior,
				"AU must be exclusive for %s/%s", plan, period)
		}
	}
}

func TestCatalog_NonAUCurrenciesHaveNoTaxBehavior(t *testing.T) {
	opts, _ := pricing.DevelopedCurrencyOptions(pricing.PlanStarter, pricing.PeriodMonthly)
	for cur, amt := range opts {
		if cur == "aud" {
			continue
		}
		require.Empty(t, amt.TaxBehavior, "currency %s should not set TaxBehavior", cur)
	}
}

// ---------------------------------------------------------------------------
// Pro monthly 20% premium over annual-equivalent
// ---------------------------------------------------------------------------

func TestCatalog_ProMonthlyPremium_20Percent(t *testing.T) {
	annualMo := pricing.MustGet(pricing.PlanPro, pricing.PeriodAnnual, "usd").UnitAmountMinor / 12
	monthly := pricing.MustGet(pricing.PlanPro, pricing.PeriodMonthly, "usd").UnitAmountMinor
	ratio := float64(monthly) / float64(annualMo)
	require.InDelta(t, 1.20, ratio, 0.01, "Pro monthly must be ~20%% premium over annual-equivalent")
}

func TestCatalog_ProMonthlyPremium_CAD(t *testing.T) {
	annualMo := pricing.MustGet(pricing.PlanPro, pricing.PeriodAnnual, "cad").UnitAmountMinor / 12
	monthly := pricing.MustGet(pricing.PlanPro, pricing.PeriodMonthly, "cad").UnitAmountMinor
	ratio := float64(monthly) / float64(annualMo)
	require.InDelta(t, 1.20, ratio, 0.02)
}

// ---------------------------------------------------------------------------
// Spec-specific value checks (catch divergence from defaults)
// ---------------------------------------------------------------------------

func TestCatalog_SpecValues_StarterAnnualCAD_239(t *testing.T) {
	opts, _ := pricing.DevelopedCurrencyOptions(pricing.PlanStarter, pricing.PeriodAnnual)
	// Spec says C$239 (task default incorrectly said C$240)
	require.Equal(t, int64(23900), opts["cad"].UnitAmountMinor)
}

func TestCatalog_SpecValues_StarterAnnualNZD_278(t *testing.T) {
	opts, _ := pricing.DevelopedCurrencyOptions(pricing.PlanStarter, pricing.PeriodAnnual)
	// Spec says NZ$278 (task default said NZ$288 — different)
	require.Equal(t, int64(27800), opts["nzd"].UnitAmountMinor)
}

func TestCatalog_SpecValues_StudioAnnualSGD_623(t *testing.T) {
	opts, _ := pricing.DevelopedCurrencyOptions(pricing.PlanStudio, pricing.PeriodAnnual)
	// Spec says S$623 (task default said S$624)
	require.Equal(t, int64(62300), opts["sgd"].UnitAmountMinor)
}

func TestCatalog_SpecValues_PHPStarterMonthly_749(t *testing.T) {
	p, _ := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "php")
	// Spec says ₱749 (task default said ₱899)
	require.Equal(t, int64(74900), p.UnitAmountMinor)
}

func TestCatalog_SpecValues_IDRStarterMonthly_199000(t *testing.T) {
	p, _ := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "idr")
	// Spec says Rp199,000 (task default said Rp229,000); ×100 for Stripe = 19,900,000
	require.Equal(t, int64(19900000), p.UnitAmountMinor)
}

func TestCatalog_SpecValues_VNDStarterMonthly_329000(t *testing.T) {
	p, _ := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "vnd")
	// Spec says ₫329,000 (task default said ₫399,000); ×100 for Stripe = 32,900,000
	require.Equal(t, int64(32900000), p.UnitAmountMinor)
}

// ---------------------------------------------------------------------------
// AllDescriptors and LookupKey invariants
// ---------------------------------------------------------------------------

func TestCatalog_AllDescriptors_HaveLookupKeys(t *testing.T) {
	for _, d := range pricing.AllDescriptors() {
		require.NotEmpty(t, d.LookupKey,
			"%s/%s/%s/%s must have LookupKey", d.Plan, d.Period, d.Tier, d.Currency)
	}
}

func TestCatalog_AllDescriptors_LookupKeysContainV1Suffix(t *testing.T) {
	for _, d := range pricing.AllDescriptors() {
		require.True(t, strings.HasSuffix(d.LookupKey, "_v1"),
			"LookupKey %q must end with _v1 for versioned bootstrap idempotency", d.LookupKey)
	}
}

func TestCatalog_AllDescriptors_LookupKeysContainMark8lyPrefix(t *testing.T) {
	for _, d := range pricing.AllDescriptors() {
		require.True(t, strings.HasPrefix(d.LookupKey, "mark8ly_"),
			"LookupKey %q must start with mark8ly_", d.LookupKey)
	}
}

func TestCatalog_DevelopedDescriptorCount(t *testing.T) {
	// 3 plans × 2 periods = 6 developed descriptors
	count := 0
	for _, d := range pricing.AllDescriptors() {
		if d.Tier == pricing.TierDeveloped {
			count++
		}
	}
	require.Equal(t, 6, count, "expected 6 developed-market descriptors (3 plans × 2 periods)")
}

func TestCatalog_PPPDescriptorCount(t *testing.T) {
	// (Starter monthly + Starter annual + Studio monthly + Studio annual + Pro annual + Pro monthly)
	// × 6 currencies = 36 PPP descriptors
	count := 0
	for _, d := range pricing.AllDescriptors() {
		if d.Tier == pricing.TierPPP {
			count++
		}
	}
	require.Equal(t, 36, count, "expected 36 PPP descriptors (6 plan/period combos × 6 currencies)")
}

func TestCatalog_AllDescriptors_PPPOptionsHaveOneCurrency(t *testing.T) {
	for _, d := range pricing.AllDescriptors() {
		if d.Tier != pricing.TierPPP {
			continue
		}
		require.Len(t, d.Options, 1,
			"PPP descriptor %s must have exactly 1 Options entry (separate Price per currency)", d.LookupKey)
	}
}

func TestCatalog_MustGetDescriptor_DevelopedStarter(t *testing.T) {
	d := pricing.MustGetDescriptor(pricing.PlanStarter, pricing.PeriodMonthly, pricing.TierDeveloped)
	require.Equal(t, "mark8ly_starter_monthly_developed_v1", d.LookupKey)
}

func TestCatalog_MustGetDescriptor_PanicsOnMiss(t *testing.T) {
	require.Panics(t, func() {
		pricing.MustGetDescriptor("nonexistent", pricing.PeriodMonthly, pricing.TierDeveloped)
	})
}

func TestCatalog_AllDescriptors_ImmutableCopy(t *testing.T) {
	first := pricing.AllDescriptors()
	second := pricing.AllDescriptors()
	require.Equal(t, len(first), len(second), "AllDescriptors must return consistent results")
	// Mutating the returned slice must not affect subsequent calls.
	first[0] = pricing.PriceDescriptor{}
	third := pricing.AllDescriptors()
	require.NotEmpty(t, third[0].LookupKey, "AllDescriptors must return an independent copy")
}
