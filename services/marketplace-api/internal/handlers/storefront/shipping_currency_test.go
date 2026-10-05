package storefront

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// #1007. DHL quoted £178.95 for an Australian domestic shipment on an
// AUD store, and the buyer was charged A$178.95 — the same number in a
// currency worth about half as much. The merchant ate the difference on
// every order that picked one of those rates.
//
// Nothing anywhere compared the rate's currency to the order's. These
// pin that it now does, in both directions: a foreign rate must never be
// priced, and a legitimate one must not be dropped by an over-eager
// guard that leaves a store unable to ship at all.

func rate(service, currency, price string) shipping.Rate {
	return shipping.Rate{
		Service:      service,
		Carrier:      "dhl_express",
		Price:        decimal.RequireFromString(price),
		CurrencyCode: currency,
	}
}

func TestRatesInCurrency_DropsAForeignQuote(t *testing.T) {
	out := ratesInCurrency([]shipping.Rate{
		rate("express", "GBP", "178.95"),
		rate("standard", "AUD", "22.40"),
	}, "AUD")

	require.Len(t, out, 1)
	require.Equal(t, "standard", out[0].Service)
}

func TestRatesInCurrency_KeepsARateWithNoCurrency(t *testing.T) {
	// The flat-rate and store-configured paths build rates without a
	// currency and are store-currency by construction. Dropping those
	// would leave stores with no shipping at all.
	out := ratesInCurrency([]shipping.Rate{rate("flat", "", "9.95")}, "AUD")
	require.Len(t, out, 1)
}

func TestRatesInCurrency_IsCaseInsensitive(t *testing.T) {
	out := ratesInCurrency([]shipping.Rate{rate("express", "aud", "10")}, "AUD")
	require.Len(t, out, 1)
}

func TestRatesInCurrency_UnknownStoreCurrencyKeepsEverything(t *testing.T) {
	// Refusing to price anything because the STORE's currency is
	// missing would be a worse failure than the one being fixed.
	out := ratesInCurrency([]shipping.Rate{rate("express", "GBP", "178.95")}, "")
	require.Len(t, out, 1)
}

// The money assertion. Before this, selectShippingPrice returned
// 178.95 here and the caller added it to an AUD order.
func TestSelectShippingPrice_RefusesToPriceAForeignRate(t *testing.T) {
	_, ok := selectShippingPrice(
		[]shipping.Rate{rate("express", "GBP", "178.95")},
		"express", decimal.Zero, nil, decimal.NewFromInt(50), "AUD")

	require.False(t, ok,
		"a GBP rate must not price an AUD order; the caller falls back instead")
}

func TestSelectShippingPrice_PicksTheRequestedRateInStoreCurrency(t *testing.T) {
	price, ok := selectShippingPrice(
		[]shipping.Rate{
			rate("express", "GBP", "178.95"),
			rate("standard", "AUD", "22.40"),
		},
		"standard", decimal.Zero, nil, decimal.NewFromInt(50), "AUD")

	require.True(t, ok)
	require.Equal(t, "22.40", price.StringFixed(2))
}

func TestSelectShippingPrice_DoesNotFallBackOntoAForeignRate(t *testing.T) {
	// The fallback is "first rate" when the requested service is absent.
	// It must not reach past the guard and grab the GBP one.
	_, ok := selectShippingPrice(
		[]shipping.Rate{rate("express", "GBP", "178.95")},
		"a-service-that-does-not-exist", decimal.Zero, nil, decimal.NewFromInt(50), "AUD")

	require.False(t, ok)
}

func TestSelectShippingPrice_StillAddsHandlingAndHonoursFreeShipping(t *testing.T) {
	// The guard must not disturb the existing pricing rules.
	price, ok := selectShippingPrice(
		[]shipping.Rate{rate("standard", "AUD", "10.00")},
		"standard", decimal.RequireFromString("2.50"), nil, decimal.NewFromInt(50), "AUD")
	require.True(t, ok)
	require.Equal(t, "12.50", price.StringFixed(2))

	min := decimal.NewFromInt(40)
	free, ok := selectShippingPrice(
		[]shipping.Rate{rate("standard", "AUD", "10.00")},
		"standard", decimal.RequireFromString("2.50"), &min, decimal.NewFromInt(50), "AUD")
	require.True(t, ok)
	require.True(t, free.IsZero())
}
