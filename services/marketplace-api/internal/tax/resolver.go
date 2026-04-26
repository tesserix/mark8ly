package tax

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// NewCalculator is a factory that returns the correct Calculator for the given
// strategy string. Accepted strategies:
//
//   - "flat"      — FlatCalculator; rate must be non-nil
//   - "india_gst" — IndiaGSTCalculator; rate and taxjarAPIKey are ignored
//   - "taxjar"    — TaxJarCalculator; taxjarAPIKey is required, taxjarMode
//     selects live vs sandbox
func NewCalculator(strategy string, rate *float64, taxjarAPIKey string, taxjarMode string) (Calculator, error) {
	switch strategy {
	case "flat":
		if rate == nil {
			return nil, fmt.Errorf("tax: flat strategy requires a rate")
		}
		// supported_countries.tax_rate is stored as a percentage (10.00
		// for AU GST = 10%). FlatCalculator multiplies its internal rate
		// directly into the line / shipping totals, so we must convert
		// to a fraction here. The previous code stored the raw
		// percentage and let checkout_ext.go pre-divide the per-item
		// rates — but flat.go's shipping-tax branch uses the
		// calculator's internal rate, so a $12.95 AU shipping line was
		// being taxed at 10× (1000%) and dropping a ~$130 phantom
		// shipping tax onto every order.
		r := decimal.NewFromFloat(*rate).Div(decimal.NewFromInt(100))
		desc := fmt.Sprintf("Tax %s%%", decimal.NewFromFloat(*rate).String())
		return NewFlatCalculator(r, desc), nil

	case "india_gst":
		return NewIndiaGSTCalculator(), nil

	case "taxjar":
		if taxjarAPIKey == "" {
			return nil, fmt.Errorf("tax: taxjar strategy requires an API key")
		}
		return NewTaxJarCalculator(taxjarAPIKey, taxjarMode), nil

	default:
		return nil, fmt.Errorf("tax: unknown strategy %q", strategy)
	}
}
