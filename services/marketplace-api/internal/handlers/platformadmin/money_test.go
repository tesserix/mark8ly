package platformadmin

import (
	"testing"

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

func TestResolveMoney_UnknownPlanDoesNotPanic(t *testing.T) {
	var m money
	var ok bool
	require.NotPanics(t, func() {
		m, ok = resolveMoney("nonexistent", "monthly", strPtr("usd"), subscription.PriceTierDeveloped)
	})
	require.False(t, ok)
	require.Equal(t, money{}, m)
}
