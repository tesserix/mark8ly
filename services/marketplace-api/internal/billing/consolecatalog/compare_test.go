package consolecatalog_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// #304 / #392: before the console can replace internal/billing/pricing as the
// price source, both are read in parallel and compared on every read, logging
// differences without changing behaviour, until the difference count is
// durably zero. Same evidence pattern the console's own parity check uses.
//
// This file is the comparison. Its whole value is being trustworthy: a
// comparator that reports spurious differences trains everyone to ignore it,
// and one that misses real differences makes the cutover unsafe.

// The trap that would have produced 78 false differences on day one.
//
// The Go catalog stores "Stripe's default" as an EMPTY tax_behavior; the
// console reports the same fact as the string "unspecified". Compared
// naively, every price in the catalog differs, the difference count never
// reaches zero, and the cutover it gates never happens.
//
// Verified against the live endpoint on 2026-08-30: console rows carry
// "unspecified" where catalog.go carries "".
func TestDiff_TreatsUnspecifiedAndEmptyTaxBehaviourAsEqual(t *testing.T) {
	compiled := []pricing.PriceDescriptor{{
		Plan: "pro", Period: "annual", Tier: pricing.TierPPP,
		Currency:  "vnd",
		Baseline:  pricing.Amount{Currency: "vnd", UnitAmountMinor: 1978800000, TaxBehavior: ""},
		Options:   map[string]pricing.Amount{"vnd": {Currency: "vnd", UnitAmountMinor: 1978800000}},
		LookupKey: "mark8ly_pro_annual_ppp_vnd_v1",
	}}
	console := consolecatalog.Catalog{Prices: []consolecatalog.Price{{
		LookupKey: "mark8ly_pro_annual_ppp_vnd_v1", Plan: "pro", Period: "annual",
		Tier: "ppp", Currency: "vnd", UnitAmountMinor: 1978800000,
		TaxBehavior: "unspecified",
	}}}

	require.Empty(t, consolecatalog.Diff(console, compiled),
		`"" and "unspecified" are the same fact stated two ways — treating them as `+
			`different reports every price as divergent and the count never reaches zero`)
}

// A real amount change must be caught. This is the case the whole exercise
// exists to detect before the cutover.
func TestDiff_ReportsAChangedAmount(t *testing.T) {
	compiled := []pricing.PriceDescriptor{{
		Plan: "pro", Period: "monthly", Tier: pricing.TierPPP, Currency: "inr",
		Baseline:  pricing.Amount{Currency: "inr", UnitAmountMinor: 100000},
		Options:   map[string]pricing.Amount{"inr": {Currency: "inr", UnitAmountMinor: 100000}},
		LookupKey: "mark8ly_pro_monthly_ppp_inr_v1",
	}}
	console := consolecatalog.Catalog{Prices: []consolecatalog.Price{{
		LookupKey: "mark8ly_pro_monthly_ppp_inr_v1", Plan: "pro", Period: "monthly",
		Tier: "ppp", Currency: "inr", UnitAmountMinor: 123456,
	}}}

	diffs := consolecatalog.Diff(console, compiled)
	require.Len(t, diffs, 1)
	require.Equal(t, "mark8ly_pro_monthly_ppp_inr_v1", diffs[0].LookupKey)
	require.Equal(t, "inr", diffs[0].Currency)
	require.Contains(t, diffs[0].Detail, "100000")
	require.Contains(t, diffs[0].Detail, "123456")
}

// A developed-tier descriptor is ONE Price object carrying an Options map of
// seven currencies, and the console returns one row per currency. Comparing
// only the baseline would leave six of every seven amounts unchecked — the
// same currency_options blind spot #459 had to leave open on the Stripe side,
// which is worth closing HERE because the data is right in front of us.
func TestDiff_ComparesEveryCurrencyOfADevelopedPrice(t *testing.T) {
	compiled := []pricing.PriceDescriptor{{
		Plan: "starter", Period: "monthly", Tier: pricing.TierDeveloped,
		Currency: "usd",
		Baseline: pricing.Amount{Currency: "usd", UnitAmountMinor: 1900},
		Options: map[string]pricing.Amount{
			"usd": {Currency: "usd", UnitAmountMinor: 1900},
			"gbp": {Currency: "gbp", UnitAmountMinor: 1500},
		},
		LookupKey: "mark8ly_starter_monthly_developed_v1",
	}}
	console := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "mark8ly_starter_monthly_developed_v1", Currency: "usd", UnitAmountMinor: 1900, Tier: "developed"},
		{LookupKey: "mark8ly_starter_monthly_developed_v1", Currency: "gbp", UnitAmountMinor: 9999, Tier: "developed"},
	}}

	diffs := consolecatalog.Diff(console, compiled)
	require.Len(t, diffs, 1, "the non-baseline currency must be compared, not just the baseline")
	require.Equal(t, "gbp", diffs[0].Currency)
}

// A row present on one side only is a difference, and the two directions are
// reported distinctly: a price the console has never heard of and one it has
// added are different problems with different fixes.
func TestDiff_ReportsRowsMissingFromEitherSide(t *testing.T) {
	compiled := []pricing.PriceDescriptor{{
		Plan: "pro", Period: "annual", Tier: pricing.TierPPP, Currency: "thb",
		Baseline:  pricing.Amount{Currency: "thb", UnitAmountMinor: 500},
		Options:   map[string]pricing.Amount{"thb": {Currency: "thb", UnitAmountMinor: 500}},
		LookupKey: "only_in_catalog",
	}}
	console := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "only_in_console", Currency: "thb", UnitAmountMinor: 500, Tier: "ppp"},
	}}

	diffs := consolecatalog.Diff(console, compiled)
	require.Len(t, diffs, 2)
	byKey := map[string]consolecatalog.Difference{}
	for _, d := range diffs {
		byKey[d.LookupKey] = d
	}
	require.Contains(t, byKey["only_in_catalog"].Detail, "absent from the console")
	require.Contains(t, byKey["only_in_console"].Detail, "absent from the compiled catalog")
}

// Identical inputs must be silent. Stated explicitly because "durably zero"
// is the cutover gate, and a comparator with a floor above zero can never
// reach it.
func TestDiff_IdenticalSourcesProduceNoDifferences(t *testing.T) {
	compiled := pricing.AllDescriptors()
	var prices []consolecatalog.Price
	for _, d := range compiled {
		for cur, amt := range d.Options {
			if amt.Currency == "" {
				continue // catalog gap, not a real price — see money.go
			}
			prices = append(prices, consolecatalog.Price{
				LookupKey: d.LookupKey, Plan: string(d.Plan), Period: string(d.Period),
				Tier: string(d.Tier), Currency: cur, UnitAmountMinor: amt.UnitAmountMinor,
				TaxBehavior: amt.TaxBehavior,
			})
		}
	}
	require.NotEmpty(t, prices, "a vacuous comparison would prove nothing")
	require.Empty(t, consolecatalog.Diff(consolecatalog.Catalog{Prices: prices}, compiled))
}
