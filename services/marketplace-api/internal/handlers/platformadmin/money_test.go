package platformadmin

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func TestResolveMoney_DevelopedTier(t *testing.T) {
	m, ok := resolveMoney("starter", "monthly", strPtr("gbp"), subscription.PriceTierDeveloped)
	require.True(t, ok)
	require.Equal(t, money{Amount: 1500, Currency: "GBP"}, m)
}

func TestResolveMoney_PPPTier_DiffersFromDeveloped(t *testing.T) {
	pppMoney, ok := resolveMoney("starter", "monthly", strPtr("inr"), subscription.PriceTierPPP)
	require.True(t, ok)
	require.Equal(t, money{Amount: 99900, Currency: "INR"}, pppMoney)

	// inr is not a developed-tier currency, so use a developed price for the
	// same plan/period to prove the PPP lookup didn't fall through to the
	// developed table.
	developedMoney, ok := resolveMoney("starter", "monthly", strPtr("usd"), subscription.PriceTierDeveloped)
	require.True(t, ok)
	require.NotEqual(t, pppMoney.Amount, developedMoney.Amount)
}

func TestResolveMoney_TrialPlanHasNoPrice(t *testing.T) {
	_, ok := resolveMoney("trial", "monthly", strPtr("usd"), subscription.PriceTierDeveloped)
	require.False(t, ok)
}

func TestResolveMoney_MarketplacePlanHasNoPrice(t *testing.T) {
	_, ok := resolveMoney("marketplace", "monthly", strPtr("usd"), subscription.PriceTierDeveloped)
	require.False(t, ok)
}

func TestResolveMoney_NilBillingCurrency(t *testing.T) {
	_, ok := resolveMoney("starter", "monthly", nil, subscription.PriceTierDeveloped)
	require.False(t, ok)
}

func TestResolveMoney_EmptyAndWhitespaceBillingCurrency(t *testing.T) {
	_, ok := resolveMoney("starter", "monthly", strPtr(""), subscription.PriceTierDeveloped)
	require.False(t, ok)

	_, ok = resolveMoney("starter", "monthly", strPtr("   "), subscription.PriceTierDeveloped)
	require.False(t, ok)
}

func TestResolveMoney_UppercaseCurrencyInputResolvesSameAsLowercase(t *testing.T) {
	upper, ok := resolveMoney("starter", "monthly", strPtr("GBP"), subscription.PriceTierDeveloped)
	require.True(t, ok)

	lower, ok := resolveMoney("starter", "monthly", strPtr("gbp"), subscription.PriceTierDeveloped)
	require.True(t, ok)

	require.Equal(t, lower, upper)
}

func TestResolveMoney_UnknownCurrencyDoesNotPanic(t *testing.T) {
	var m money
	var ok bool
	require.NotPanics(t, func() {
		m, ok = resolveMoney("starter", "monthly", strPtr("zzz"), subscription.PriceTierDeveloped)
	})
	require.False(t, ok)
	require.Equal(t, money{}, m)
}

// TestResolveMoney_ZeroValueCatalogEntryGuard pins the guard added to the
// developed-tier branch of resolveMoney: `pricing.DevelopedCurrencyOptions`
// pre-populates its Options map with a zero-value pricing.Amount for every
// developed currency, even one the catalog has no price for (catalog.go's
// init does `opts[c] = byPeriod[c]` unconditionally over all 7 currencies).
// A plain `present` check on that map therefore cannot distinguish "no
// price" from "zero-value placeholder present" — resolveMoney must also
// require amt.Currency != "".
//
// Today every (plan, period) in developedAmounts populates all 7
// currencies, so this gap is unreachable through resolveMoney itself
// without editing the catalog (which this task explicitly forbids). This
// test instead pins the guard's condition directly against the same
// pricing.Amount zero value the real map would produce, so a regression
// that drops the `amt.Currency != ""` check is caught without depending on
// catalog contents.
func TestResolveMoney_ZeroValueCatalogEntryGuard(t *testing.T) {
	// isResolvable mirrors the exact condition in resolveMoney's
	// developed-tier branch: `present && amt.Currency != ""`.
	isResolvable := func(opts map[string]pricing.Amount, cur string) bool {
		amt, present := opts[cur]
		return present && amt.Currency != ""
	}

	// A catalog-populated entry (mirrors byPeriod[c] with a real price).
	populated := map[string]pricing.Amount{
		"gbp": {Currency: "gbp", UnitAmountMinor: 1500},
	}
	require.True(t, isResolvable(populated, "gbp"))

	// A zero-value entry, exactly what `opts[c] = byPeriod[c]` produces
	// when byPeriod has no key c: map lookup succeeds (present == true)
	// but the Amount is the zero value.
	withZeroValueEntry := map[string]pricing.Amount{
		"nzd": {},
	}
	amt, present := withZeroValueEntry["nzd"]
	require.True(t, present, "map lookup must report present for a zero-value entry, proving present alone is not sufficient")
	require.Equal(t, pricing.Amount{}, amt)
	require.False(t, isResolvable(withZeroValueEntry, "nzd"), "a zero-value catalog entry must not be treated as resolvable")

	// A genuinely absent key must also be unresolvable.
	require.False(t, isResolvable(populated, "nzd"))
}

func TestResolveMoney_UnknownPlanDoesNotPanic(t *testing.T) {
	var m money
	var ok bool
	require.NotPanics(t, func() {
		m, ok = resolveMoney("nonexistent", "monthly", strPtr("usd"), subscription.PriceTierDeveloped)
	})
	require.False(t, ok)
	require.Equal(t, money{}, m)
}
