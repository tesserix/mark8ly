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
		r := decimal.NewFromFloat(*rate)
		pctLabel := r.Mul(decimal.NewFromInt(100)).String()
		desc := fmt.Sprintf("Tax %s%%", pctLabel)
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
