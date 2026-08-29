package stripe

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// The zero-decimal trap, which is why DescribeMismatch lives beside
// stripeUnitAmount rather than in the bootstrap package (#459).
//
// The catalog stores zero-decimal currencies pre-multiplied by 100 and the
// conversion is applied on the way into Stripe. A comparison written
// anywhere else has to re-implement that conversion — and a first draft of
// this check, written in the bootstrap package, did not: it compared the
// raw catalog field and declared every VND price divergent by a factor of
// 100, which would have refused every bootstrap run outright.
func TestDescribeMismatch_ZeroDecimalCurrencyIsNotAFalsePositive(t *testing.T) {
	var vnd *pricing.PriceDescriptor
	for _, d := range pricing.AllDescriptors() {
		if d.Baseline.Currency == "vnd" {
			cp := d
			vnd = &cp
			break
		}
	}
	require.NotNil(t, vnd, "expected a VND descriptor in the catalog")

	// What bootstrap actually wrote to Stripe for this descriptor.
	stored := &Price{
		UnitAmount:  stripeUnitAmount(vnd.Baseline.Currency, vnd.Baseline.UnitAmountMinor),
		Currency:    vnd.Baseline.Currency,
		TaxBehavior: vnd.Baseline.TaxBehavior,
	}
	require.Equal(t, vnd.Baseline.UnitAmountMinor/100, stored.UnitAmount,
		"fixture check: VND must actually be divided, or this test proves nothing")

	require.Empty(t, DescribeMismatch(*vnd, stored),
		"a correctly bootstrapped zero-decimal price must not read as divergent")
}

// The case the refusal exists for.
func TestDescribeMismatch_ReportsAChangedAmount(t *testing.T) {
	d := pricing.AllDescriptors()[0]
	stored := &Price{
		UnitAmount:  stripeUnitAmount(d.Baseline.Currency, d.Baseline.UnitAmountMinor) + 500,
		Currency:    d.Baseline.Currency,
		TaxBehavior: d.Baseline.TaxBehavior,
	}

	got := DescribeMismatch(d, stored)
	require.NotEmpty(t, got, "a changed amount must be reported, not silently reused")
	require.Contains(t, got, "unit_amount")
}

// A nil existing price means "not found", which is the create path, not a
// divergence.
func TestDescribeMismatch_NilExistingIsNotADivergence(t *testing.T) {
	require.Empty(t, DescribeMismatch(pricing.AllDescriptors()[0], nil))
}
