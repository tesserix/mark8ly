package pricing_test

// The Pro+App add-on's annual price is written down twice in Go, in packages
// that have no relationship to each other:
//
//   - pricing.ProAppAnnual  — the DISPLAY figure, from which
//     cmd/genpricing generates the TypeScript catalogue the admin and
//     onboarding apps render.
//   - appaddon.AppAnnualCents — the BILLING figure, which appaddon.ProrationCents
//     multiplies by the remaining fraction of the anchor year to compute the
//     amount actually invoiced.
//
// Nothing imported one from the other and nothing asserted they agreed, so a
// third hand-written copy in the admin purchase page was free to disagree with
// both (tesserix/mark8ly#650) and did. Reconciling the copies removes that
// third one; this test closes the drift between the two that remain.
//
// This lives in the EXTERNAL test package `pricing_test` on purpose. `pricing`
// and `appaddon` import each other in neither direction today, and asserting
// the two constants agree must not be what creates that edge. An external test
// package is never imported by anything, so it can depend on both packages
// without any possibility of an import cycle — and it stays cycle-proof even
// if a future change makes one of the two packages import the other.

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/billing/appaddon"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/stretchr/testify/require"
)

// TestProAppAnnualMatchesBillingConstant pins the display figure to the figure
// that is actually charged. If these ever diverge, a merchant is quoted one
// number on the pricing page and invoiced another.
func TestProAppAnnualMatchesBillingConstant(t *testing.T) {
	require.Equal(t, appaddon.AppAnnualCents, pricing.ProAppAnnual.UnitAmountMinor,
		"pricing.ProAppAnnual (displayed) must equal appaddon.AppAnnualCents (billed)")
}

// TestProAppMonthlyTimesTwelveIsAnnual pins the internal consistency of the
// display pair: spec §3.4 defines the annual figure as twelve monthly charges
// ($199 × 12 = $2,388) — there is no annual discount on the add-on.
func TestProAppMonthlyTimesTwelveIsAnnual(t *testing.T) {
	require.Equal(t, pricing.ProAppAnnual.UnitAmountMinor, pricing.ProAppMonthly.UnitAmountMinor*12,
		"spec §3.4: the add-on's annual price is exactly 12 × the monthly price, with no annual discount")
	require.Equal(t, pricing.ProAppMonthly.Currency, pricing.ProAppAnnual.Currency,
		"both add-on amounts are denominated in the same currency (spec §4.1.2: USD only)")
}

// TestProAppAnnualIsNotTheSetupFee guards the specific confusion behind #650:
// $2,000 is the ONE-TIME setup charge, not a full year of add-on fees. The two
// are added together by appaddon.ProrationCents at purchase, which is why a
// purchase-time total and a full-year price are legitimately different numbers.
func TestProAppAnnualIsNotTheSetupFee(t *testing.T) {
	require.NotEqual(t, appaddon.SetupFeeCents, pricing.ProAppAnnual.UnitAmountMinor,
		"the annual add-on price must never be the one-time setup fee")
}
