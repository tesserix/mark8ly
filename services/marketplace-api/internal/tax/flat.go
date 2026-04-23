package tax

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// FlatCalculator applies a country-default tax rate to line items,
// respecting per-item overrides and category exemptions. Used by
// countries with a uniform VAT/GST rate: GB, DE, FR, IT, ES, NL, AU,
// CA, SG, MY, TH, PH, ID.
//
// Per-item behaviour:
//   - TaxCategory == exempt / zero_rated → item contributes 0 tax
//   - TaxRate > 0                        → use the override rate
//   - otherwise                          → use the calculator's default
//
// Output groups tax lines by distinct rate so a cart with mixed
// standard and reduced-rate items produces two "VAT 20%" and
// "VAT 10%" lines.
type FlatCalculator struct {
	rate        decimal.Decimal
	description string // e.g. "VAT", "GST"
}

// NewFlatCalculator returns a calculator with rate as the country
// default. description is the human-readable tax-line prefix
// (e.g. "VAT", "GST"). rate is a fraction (0.20 for 20%).
func NewFlatCalculator(rate decimal.Decimal, description string) *FlatCalculator {
	return &FlatCalculator{
		rate:        rate,
		description: description,
	}
}

func (c *FlatCalculator) ProviderName() string { return "flat" }

func (c *FlatCalculator) Calculate(_ context.Context, in TaxRequest) (*TaxBreakdown, error) {
	if len(in.Items) == 0 {
		return &TaxBreakdown{TaxTotal: decimal.Zero}, nil
	}

	// Aggregate item tax by effective rate so we emit one line per
	// distinct rate seen across the cart.
	byRate := map[string]*struct {
		rate   decimal.Decimal
		amount decimal.Decimal
	}{}

	taxableItemsSubtotal := decimal.Zero

	for _, item := range in.Items {
		if item.IsExempt() {
			continue
		}
		rate := c.rate
		if item.TaxRate.GreaterThan(decimal.Zero) {
			rate = item.TaxRate
		}
		if rate.IsZero() {
			continue
		}
		lineTotal := item.Amount.Mul(decimal.NewFromInt(int64(item.Quantity)))
		taxableItemsSubtotal = taxableItemsSubtotal.Add(lineTotal)
		lineTax := lineTotal.Mul(rate).Round(2)
		key := rate.String()
		if a, ok := byRate[key]; ok {
			a.amount = a.amount.Add(lineTax)
		} else {
			byRate[key] = &struct {
				rate   decimal.Decimal
				amount decimal.Decimal
			}{rate: rate, amount: lineTax}
		}
	}

	total := decimal.Zero
	lines := make([]TaxLine, 0, len(byRate)+1)
	for _, a := range byRate {
		pct := a.rate.Mul(decimal.NewFromInt(100)).String()
		lines = append(lines, TaxLine{
			Description: fmt.Sprintf("%s %s%% on items", c.description, pct),
			Rate:        a.rate,
			Amount:      a.amount,
		})
		total = total.Add(a.amount)
	}

	// Shipping always taxed at the country-default rate when any items
	// in the cart are taxable. If every item is exempt, shipping is too.
	if in.ShippingAmount.GreaterThan(decimal.Zero) &&
		taxableItemsSubtotal.GreaterThan(decimal.Zero) &&
		c.rate.GreaterThan(decimal.Zero) {
		shippingTax := in.ShippingAmount.Mul(c.rate).Round(2)
		pct := c.rate.Mul(decimal.NewFromInt(100)).String()
		lines = append(lines, TaxLine{
			Description: fmt.Sprintf("%s %s%% on shipping", c.description, pct),
			Rate:        c.rate,
			Amount:      shippingTax,
		})
		total = total.Add(shippingTax)
	}

	return &TaxBreakdown{
		TaxTotal: total,
		Lines:    lines,
	}, nil
}
