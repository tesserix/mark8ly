package checkoutcore

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/country"
)

func TestTaxableItems_RatesArePercentagesConvertedToFractions(t *testing.T) {
	countryRate := 10.0
	sc := &country.SupportedCountry{TaxRate: &countryRate}
	override := decimal.NewFromInt(18)
	pid := "p-1"
	code := "8471"

	got := TaxableItems(sc, []TaxLine{
		{SKU: "A", ProductID: &pid, UnitPrice: decimal.NewFromInt(110), Quantity: 2},
		{SKU: "B", UnitPrice: decimal.NewFromInt(50), Quantity: 1, TaxRateOverride: &override, TaxCode: &code},
	})

	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if !got[0].TaxRate.Equal(decimal.RequireFromString("0.1")) || got[0].ProductID != "p-1" || got[0].Quantity != 2 {
		t.Errorf("country default line = %+v, want rate 0.1, product p-1, qty 2", got[0])
	}
	if !got[1].TaxRate.Equal(decimal.RequireFromString("0.18")) || got[1].TaxCode != "8471" {
		t.Errorf("override line = %+v, want rate 0.18 and tax code 8471", got[1])
	}
}

func TestTaxableItems_NoRateAnywhereIsZero(t *testing.T) {
	got := TaxableItems(&country.SupportedCountry{}, []TaxLine{{SKU: "A", UnitPrice: decimal.NewFromInt(5), Quantity: 1}})
	if !got[0].TaxRate.IsZero() {
		t.Errorf("rate = %s, want 0", got[0].TaxRate)
	}
}
