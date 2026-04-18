// Command pricing-dump writes the full pricing catalog to stdout as CSV.
//
// Usage:
//
//	go run ./cmd/pricing-dump > pricing-v1.csv
//
// Output columns: plan, period, tier, currency, unit_amount_minor, tax_behavior, lookup_key
package main

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

func main() {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	if err := w.Write([]string{
		"plan", "period", "tier", "currency",
		"unit_amount_minor", "tax_behavior", "lookup_key",
	}); err != nil {
		fmt.Fprintln(os.Stderr, "pricing-dump: write header:", err)
		os.Exit(1)
	}

	for _, d := range pricing.AllDescriptors() {
		for cur, opt := range d.Options {
			if err := w.Write([]string{
				string(d.Plan),
				string(d.Period),
				string(d.Tier),
				cur,
				fmt.Sprintf("%d", opt.UnitAmountMinor),
				opt.TaxBehavior,
				d.LookupKey,
			}); err != nil {
				fmt.Fprintln(os.Stderr, "pricing-dump: write row:", err)
				os.Exit(1)
			}
		}
	}
}
