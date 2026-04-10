package tax

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// FlatCalculator applies a single tax rate to the entire order subtotal and
// shipping. Used by countries with a uniform VAT/GST rate: GB, DE, FR, IT,
// ES, NL, AU, CA, SG, MY, TH, PH, ID.
type FlatCalculator struct {
	rate        decimal.Decimal
	description string // e.g. "VAT 20%", "GST 10%"
}

// NewFlatCalculator returns a calculator that multiplies every taxable amount
// by rate. description is the human-readable label used on tax lines
// (e.g. "VAT 20%"). rate is expressed as a fraction, e.g. 0.20 for 20%.
func NewFlatCalculator(rate decimal.Decimal, description string) *FlatCalculator {
	return &FlatCalculator{
		rate:        rate,
		description: description,
	}
}

func (c *FlatCalculator) ProviderName() string { return "flat" }

func (c *FlatCalculator) Calculate(_ context.Context, in TaxRequest) (*TaxBreakdown, error) {
	if len(in.Items) == 0 {
		return &TaxBreakdown{
			TaxTotal: decimal.Zero,
			Lines:    nil,
		}, nil
	}

	subtotal := decimal.Zero
	for _, item := range in.Items {
		lineTotal := item.Amount.Mul(decimal.NewFromInt(int64(item.Quantity)))
		subtotal = subtotal.Add(lineTotal)
	}

	itemTax := subtotal.Mul(c.rate).Round(2)

	lines := []TaxLine{
		{
			Description: fmt.Sprintf("%s on items", c.description),
			Rate:        c.rate,
			Amount:      itemTax,
		},
	}

	total := itemTax

	if in.ShippingAmount.GreaterThan(decimal.Zero) {
		shippingTax := in.ShippingAmount.Mul(c.rate).Round(2)
		lines = append(lines, TaxLine{
			Description: fmt.Sprintf("%s on shipping", c.description),
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
