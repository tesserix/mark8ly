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

// DiscountTaxableItems lowers each item's taxable price by its share of an
// order-wide discount (coupon or loyalty points), so tax is on what the
// buyer pays (#1127). Shipping is untouched; gift cards are payment, not a
// discount, and are not passed here.
//
// shortcut: a coupon limited to some products is still spread over every
// line; upgrade when per-line coupon amounts are recorded.
func DiscountTaxableItems(items []tax.TaxableItem, discount decimal.Decimal) []tax.TaxableItem {
	if !discount.IsPositive() {
		return items
	}
	total := decimal.Zero
	for _, it := range items {
		total = total.Add(it.Amount.Mul(decimal.NewFromInt(int64(it.Quantity))))
	}
	if !total.IsPositive() {
		return items
	}
	share := decimal.Max(decimal.Zero, total.Sub(discount)).Div(total)
	out := make([]tax.TaxableItem, len(items))
	for i, it := range items {
		it.Amount = it.Amount.Mul(share)
		out[i] = it
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
