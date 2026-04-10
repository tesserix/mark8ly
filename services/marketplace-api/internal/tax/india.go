package tax

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// IndiaGSTCalculator computes GST per-item using each product's GSTRate field.
// Intra-state sales (same Region) split into CGST + SGST; inter-state sales
// produce a single IGST line. Shipping is NOT taxed under Indian GST rules.
type IndiaGSTCalculator struct{}

// NewIndiaGSTCalculator returns a stateless India GST calculator. Rates come
// from each TaxableItem.GSTRate field (0, 0.05, 0.12, 0.18, or 0.28).
func NewIndiaGSTCalculator() *IndiaGSTCalculator {
	return &IndiaGSTCalculator{}
}

func (c *IndiaGSTCalculator) ProviderName() string { return "india_gst" }

// taxLineKey groups tax lines by rate and type for aggregation.
type taxLineKey struct {
	taxType string // "CGST", "SGST", "IGST"
	rate    string // decimal string for map key stability
}

func (c *IndiaGSTCalculator) Calculate(_ context.Context, in TaxRequest) (*TaxBreakdown, error) {
	if len(in.Items) == 0 {
		return &TaxBreakdown{TaxTotal: decimal.Zero}, nil
	}

	sellerRegion := normaliseRegion(in.SellerAddress.Region)
	buyerRegion := normaliseRegion(in.BuyerAddress.Region)
	intraState := sellerRegion == buyerRegion && sellerRegion != ""

	// Accumulate amounts by (taxType, rate) to produce grouped lines.
	type accum struct {
		description  string
		rate         decimal.Decimal
		amount       decimal.Decimal
		jurisdiction string
	}
	grouped := map[taxLineKey]*accum{}

	two := decimal.NewFromInt(2)

	for _, item := range in.Items {
		if item.GSTRate.IsZero() {
			continue
		}

		lineTotal := item.Amount.Mul(decimal.NewFromInt(int64(item.Quantity)))

		if intraState {
			halfRate := item.GSTRate.Div(two)
			halfTax := lineTotal.Mul(halfRate).Round(2)

			for _, taxType := range []string{"CGST", "SGST"} {
				key := taxLineKey{taxType: taxType, rate: halfRate.String()}
				if a, ok := grouped[key]; ok {
					a.amount = a.amount.Add(halfTax)
				} else {
					pctLabel := halfRate.Mul(decimal.NewFromInt(100)).String()
					grouped[key] = &accum{
						description:  fmt.Sprintf("%s %s%%", taxType, pctLabel),
						rate:         halfRate,
						amount:       halfTax,
						jurisdiction: in.BuyerAddress.Region,
					}
				}
			}
		} else {
			igstTax := lineTotal.Mul(item.GSTRate).Round(2)
			key := taxLineKey{taxType: "IGST", rate: item.GSTRate.String()}
			if a, ok := grouped[key]; ok {
				a.amount = a.amount.Add(igstTax)
			} else {
				pctLabel := item.GSTRate.Mul(decimal.NewFromInt(100)).String()
				grouped[key] = &accum{
					description:  fmt.Sprintf("IGST %s%%", pctLabel),
					rate:         item.GSTRate,
					amount:       igstTax,
					jurisdiction: in.BuyerAddress.Region,
				}
			}
		}
	}

	total := decimal.Zero
	lines := make([]TaxLine, 0, len(grouped))
	for _, a := range grouped {
		lines = append(lines, TaxLine{
			Description:  a.description,
			Rate:         a.rate,
			Amount:       a.amount,
			Jurisdiction: a.jurisdiction,
		})
		total = total.Add(a.amount)
	}

	return &TaxBreakdown{
		TaxTotal: total,
		Lines:    lines,
	}, nil
}

// normaliseRegion lowercases and trims whitespace so "Maharashtra" and
// "maharashtra" compare equal.
func normaliseRegion(r string) string {
	return strings.ToLower(strings.TrimSpace(r))
}
