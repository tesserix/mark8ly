package consolecatalog_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
)

func TestIndexFindMatchesOnAllFourFields(t *testing.T) {
	ix := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "a", Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 1500},
		{LookupKey: "b", Plan: "starter", Period: "annual", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 15000},
		{LookupKey: "c", Plan: "starter", Period: "monthly", Tier: "ppp",
			Currency: "inr", UnitAmountMinor: 99900},
	}}.Index()

	require.Equal(t, 3, ix.Len())

	p, ok := ix.Find("starter", "monthly", "developed", "gbp")
	require.True(t, ok)
	require.Equal(t, int64(1500), p.UnitAmountMinor)

	p, ok = ix.Find("starter", "annual", "developed", "gbp")
	require.True(t, ok)
	require.Equal(t, int64(15000), p.UnitAmountMinor)

	p, ok = ix.Find("starter", "monthly", "ppp", "inr")
	require.True(t, ok)
	require.Equal(t, int64(99900), p.UnitAmountMinor)
}

// TestIndexFindFoldsCase covers the case mismatch that actually happens in
// production: the catalog's keys are lowercase (the console's schema
// constrains currency to lowercase ISO 4217) while the caller's currency
// comes from a char(3) column of unspecified case.
func TestIndexFindFoldsCase(t *testing.T) {
	ix := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 1500},
	}}.Index()

	for _, cur := range []string{"gbp", "GBP", "GbP", "  gbp  "} {
		_, ok := ix.Find("STARTER", " Monthly ", "Developed", cur)
		require.True(t, ok, "currency %q must match the lowercase catalog key", cur)
	}
}

// TestIndexFindMissReturnsZeroValueAndFalse is the property every caller
// depends on: absence is reported by the bool, and the Price that comes back
// with it is the zero value — so a caller that ignores the bool cannot get a
// plausible-looking amount, only an obviously empty one.
func TestIndexFindMissReturnsZeroValueAndFalse(t *testing.T) {
	ix := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 1500},
	}}.Index()

	p, ok := ix.Find("starter", "monthly", "developed", "usd")
	require.False(t, ok)
	require.Equal(t, consolecatalog.Price{}, p)
}

// TestIndexSkipsRowsWithNoCurrency pins the guard that keeps a catalog GAP
// from becoming an amount of zero. CompiledCatalog already drops these, and
// the console cannot publish one, so this pins the index's own refusal
// rather than either source's behaviour.
func TestIndexSkipsRowsWithNoCurrency(t *testing.T) {
	ix := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{Plan: "starter", Period: "monthly", Tier: "developed", Currency: ""},
		{Plan: "starter", Period: "monthly", Tier: "developed", Currency: "   "},
	}}.Index()

	require.Equal(t, 0, ix.Len(), "a row naming no currency names no price")

	_, ok := ix.Find("starter", "monthly", "developed", "")
	require.False(t, ok)
}

// TestIndexKeepsFirstOnDuplicateKey pins the tie-break so it is a decision
// rather than an accident of map iteration. Neither source is expected to
// produce a duplicate; this only says what happens if one ever does.
func TestIndexKeepsFirstOnDuplicateKey(t *testing.T) {
	ix := consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "first", Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 1500},
		{LookupKey: "second", Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "GBP", UnitAmountMinor: 9999},
	}}.Index()

	require.Equal(t, 1, ix.Len())
	p, ok := ix.Find("starter", "monthly", "developed", "gbp")
	require.True(t, ok)
	require.Equal(t, "first", p.LookupKey)
}

// TestCompiledCatalogIndexesEveryPublishedRow ties the fallback tier to the
// shape the console publishes: 42 lookup keys flatten to 78 (lookup_key,
// currency) rows, and every one of them must be reachable by
// (plan, period, tier, currency) — otherwise the rollback path would price
// some combinations and silently drop others.
func TestCompiledCatalogIndexesEveryPublishedRow(t *testing.T) {
	cat := consolecatalog.CompiledCatalog("test")
	ix := cat.Index()

	require.Equal(t, len(cat.Prices), ix.Len(),
		"every compiled row must be reachable by plan/period/tier/currency")

	for _, p := range cat.Prices {
		got, ok := ix.Find(p.Plan, p.Period, p.Tier, p.Currency)
		require.True(t, ok, "no index entry for %+v", p)
		require.Equal(t, p.UnitAmountMinor, got.UnitAmountMinor)
	}
}
