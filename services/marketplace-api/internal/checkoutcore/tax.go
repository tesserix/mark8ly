package checkoutcore

import (
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/tax"
)

// TaxLine is one priced line as the tax calculation needs it. Prices and the
// tax fields must come from the catalogue (CatalogPricer), never a client.
type TaxLine struct {
	SKU             string
	ProductID       *string
	UnitPrice       decimal.Decimal
	Quantity        int
	TaxCode         *string
	TaxCategory     *string
	TaxRateOverride *decimal.Decimal
}

// TaxableItems builds the calculator's items. Per-item fields win when set;
// otherwise the country default from supported_countries.tax_rate applies.
//
// Both sources store the rate as a PERCENTAGE (e.g. 18.00 = 18%) — the admin
// Tax tab writes percentages, and supported_countries seeds them that way.
// The calculator expects a FRACTION (0.18), so divide by 100 on the way in.
func TaxableItems(sc *country.SupportedCountry, lines []TaxLine) []tax.TaxableItem {
	hundred := decimal.NewFromInt(100)
	defaultRate := decimal.Zero
	if sc.TaxRate != nil && *sc.TaxRate > 0 {
		defaultRate = decimal.NewFromFloat(*sc.TaxRate).Div(hundred)
	}

	out := make([]tax.TaxableItem, 0, len(lines))
	for _, l := range lines {
		rate := decimal.Zero
		if l.TaxRateOverride != nil && l.TaxRateOverride.GreaterThan(decimal.Zero) {
			rate = l.TaxRateOverride.Div(hundred)
		} else if !defaultRate.IsZero() {
			rate = defaultRate
		}
		ti := tax.TaxableItem{
			SKU:         l.SKU,
			Amount:      l.UnitPrice,
			Quantity:    l.Quantity,
			TaxCode:     deref(l.TaxCode),
			TaxRate:     rate,
			TaxCategory: deref(l.TaxCategory),
		}
		if l.ProductID != nil {
			ti.ProductID = *l.ProductID
		}
		out = append(out, ti)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
