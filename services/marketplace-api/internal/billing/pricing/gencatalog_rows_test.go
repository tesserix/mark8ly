package pricing

import (
	"os"
	"strings"
	"testing"
)

// completeRows is the row list a complete console catalog produces: every
// amount in this package's own tables, restated in the flat shape the console
// serves. Stripe's default tax behaviour is written as "unspecified" here,
// which is the wording the console actually uses — confirmed against the live
// endpoint by consolecatalog's comparison — so the round trip below exercises
// the normalisation rather than skipping past it.
func completeRows() []CatalogRow {
	var rows []CatalogRow
	for _, plan := range planOrder {
		for _, period := range periodEmitOrder {
			for _, currency := range developedEmitCurrencyOrder {
				rows = append(rows, rowFor(TierDeveloped, plan, period, developedAmounts[plan][period][currency]))
			}
			for _, currency := range pppEmitCurrencyOrder {
				rows = append(rows, rowFor(TierPPP, plan, period, pppAmounts[pppKey{plan: plan, period: period, currency: currency}]))
			}
		}
	}
	return rows
}

func rowFor(tier Tier, plan Plan, period Period, amount Amount) CatalogRow {
	taxBehavior := amount.TaxBehavior
	if taxBehavior == "" {
		taxBehavior = "unspecified"
	}
	return CatalogRow{
		Plan:            string(plan),
		Period:          string(period),
		Tier:            string(tier),
		Currency:        amount.Currency,
		UnitAmountMinor: amount.UnitAmountMinor,
		TaxBehavior:     taxBehavior,
	}
}

// TestGenerateCatalogDataFromRowsMatchesCommittedFile is the mapping's real
// assertion: a complete, well-formed row list must render the committed file
// byte for byte, exactly as the literal source does.
//
// That equivalence is what makes -source=console meaningful. If a catalog
// read from the console reproduces this file, the console and the fail-open
// fallback provably held the same 78 amounts at that moment; if the row
// mapping could lose or reshape anything, the comparison would prove nothing.
func TestGenerateCatalogDataFromRowsMatchesCommittedFile(t *testing.T) {
	rows := completeRows()
	if len(rows) != expectedDevelopedRows+expectedPPPRows {
		t.Fatalf("completeRows built %d rows, want %d — the fixture, not the mapping, is wrong",
			len(rows), expectedDevelopedRows+expectedPPPRows)
	}

	generated, err := GenerateCatalogDataFromRows(rows)
	if err != nil {
		t.Fatalf("GenerateCatalogDataFromRows: %v", err)
	}

	committed, err := os.ReadFile(committedCatalogDataPath)
	if err != nil {
		t.Fatalf("reading committed %s: %v", committedCatalogDataPath, err)
	}
	if generated != string(committed) {
		t.Fatalf("rows rendered %d bytes that do not match the committed %s (%d bytes)\n%s",
			len(generated), committedCatalogDataPath, len(committed), diffPreview(generated, string(committed)))
	}
}

// TestGenerateCatalogDataFromRowsRefusesIncomplete covers the failure this
// whole path has to get right. A console that answers, but answers short,
// must not be able to write a smaller catalog_data.go: the file is what the
// serving path falls back to when the console is unreachable, so a fallback
// missing currencies would be worse than making no change at all
// (tesserix/mark8ly#631), and every other test in this package would still
// pass.
func TestGenerateCatalogDataFromRowsRefusesIncomplete(t *testing.T) {
	full := completeRows()

	cases := []struct {
		name string
		rows []CatalogRow
		want string
	}{
		{"no rows at all", nil, "78 of 78 amounts missing"},
		{"one developed currency short", withoutRow(full, TierDeveloped, PlanPro, PeriodAnnual, "eur"), "developed/pro/annual/eur"},
		{"one PPP currency short", withoutRow(full, TierPPP, PlanStarter, PeriodMonthly, "vnd"), "ppp/starter/monthly/vnd"},
		{"a whole plan short", withoutPlan(full, PlanStudio), "26 of 78 amounts missing"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := GenerateCatalogDataFromRows(tc.rows)
			if err == nil {
				t.Fatalf("generation succeeded on an incomplete catalog and emitted %d bytes; it must refuse", len(out))
			}
			if out != "" {
				t.Fatalf("generation failed but still emitted %d bytes; a partial file must never be written", len(out))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name the gap (%q), so a human cannot act on it", err, tc.want)
			}
		})
	}
}

// TestGenerateCatalogDataFromRowsRefusesUnplaceableRows covers rows the
// generator cannot place. Each is refused rather than skipped: a silently
// dropped row would land in the incomplete case above at best, and at worst
// would be a currency the console has that this file has never rendered.
func TestGenerateCatalogDataFromRowsRefusesUnplaceableRows(t *testing.T) {
	cases := []struct {
		name string
		row  CatalogRow
		want string
	}{
		{"unknown plan", CatalogRow{Plan: "enterprise", Period: "monthly", Tier: "developed", Currency: "usd"}, "unknown plan"},
		{"unknown period", CatalogRow{Plan: "pro", Period: "weekly", Tier: "developed", Currency: "usd"}, "unknown period"},
		{"unknown tier", CatalogRow{Plan: "pro", Period: "monthly", Tier: "emerging", Currency: "usd"}, "unknown tier"},
		{"currency outside the developed emit order", CatalogRow{Plan: "pro", Period: "monthly", Tier: "developed", Currency: "chf"}, "not in the developed emit order"},
		{"currency outside the PPP emit order", CatalogRow{Plan: "pro", Period: "monthly", Tier: "ppp", Currency: "brl"}, "not in the PPP emit order"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GenerateCatalogDataFromRows(append(completeRows(), tc.row)); err == nil {
				t.Fatal("generation accepted a row it cannot render; it must refuse")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not say %q", err, tc.want)
			}
		})
	}
}

// TestGenerateCatalogDataFromRowsRefusesDuplicates guards the one way a
// complete-looking catalog can still be ambiguous: two rows for the same
// amount. Last-write-wins would pass every count check while silently
// choosing between two prices.
func TestGenerateCatalogDataFromRowsRefusesDuplicates(t *testing.T) {
	full := completeRows()
	for _, tier := range []Tier{TierDeveloped, TierPPP} {
		t.Run(string(tier), func(t *testing.T) {
			dup := full[0]
			for _, r := range full {
				if r.Tier == string(tier) {
					dup = r
					break
				}
			}
			dup.UnitAmountMinor += 100
			if _, err := GenerateCatalogDataFromRows(append(append([]CatalogRow{}, full...), dup)); err == nil {
				t.Fatal("generation accepted two rows for the same amount; it must refuse")
			} else if !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("error %q does not name the duplicate", err)
			}
		})
	}
}

func withoutRow(rows []CatalogRow, tier Tier, plan Plan, period Period, currency string) []CatalogRow {
	out := make([]CatalogRow, 0, len(rows))
	for _, r := range rows {
		if r.Tier == string(tier) && r.Plan == string(plan) && r.Period == string(period) && r.Currency == currency {
			continue
		}
		out = append(out, r)
	}
	return out
}

func withoutPlan(rows []CatalogRow, plan Plan) []CatalogRow {
	out := make([]CatalogRow, 0, len(rows))
	for _, r := range rows {
		if r.Plan == string(plan) {
			continue
		}
		out = append(out, r)
	}
	return out
}
