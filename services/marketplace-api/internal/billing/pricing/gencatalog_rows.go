package pricing

import (
	"fmt"
	"sort"
	"strings"
)

// CatalogRow is one flat (plan, period, tier, currency) amount, the shape the
// console's plan-catalog route serves. It exists so cmd/gencatalog can feed
// this package a catalog read from the console without this package importing
// internal/billing/consolecatalog — that package already imports this one, and
// the dependency cannot run both ways.
//
// The console also serves a lookup_key per row. It is deliberately absent
// here: lookup-key derivation is hand-written logic in catalog.go and stays
// there, so nothing generated can disagree with it.
type CatalogRow struct {
	Plan            string
	Period          string
	Tier            string
	Currency        string
	UnitAmountMinor int64
	// TaxBehavior carries the console's wording. "unspecified" and "" both
	// mean Stripe's default and are stored as "" — the same equivalence
	// consolecatalog.sameTaxBehavior applies when comparing the two sources.
	TaxBehavior string
}

// expectedDevelopedRows and expectedPPPRows are the row counts a complete
// catalog has: every plan × period × currency of each tier. Together they are
// the 78 amounts tesserix/mark8ly#631 pinned this catalog at.
var (
	expectedDevelopedRows = len(planOrder) * len(periodEmitOrder) * len(developedEmitCurrencyOrder)
	expectedPPPRows       = len(planOrder) * len(periodEmitOrder) * len(pppEmitCurrencyOrder)
)

// GenerateCatalogDataFromRows renders catalog_data.go from a flat row list
// rather than from this package's own tables.
//
// It refuses anything short of a complete catalog. That is the point: this
// file is the fail-open fallback the serving path drops to when the console
// is unreachable, so a generation that quietly emitted fewer currencies would
// leave every other test in this package passing while removing the last
// defence against a wrong price (tesserix/mark8ly#631).
func GenerateCatalogDataFromRows(rows []CatalogRow) (string, error) {
	developed, ppp, err := tablesFromRows(rows)
	if err != nil {
		return "", err
	}
	return generateCatalogData(developed, ppp)
}

// tablesFromRows sorts the rows into the two tables catalog_data.go declares,
// rejecting any row it cannot place and any catalog that ends up incomplete.
func tablesFromRows(rows []CatalogRow) (map[Plan]map[Period]map[string]Amount, map[pppKey]Amount, error) {
	developedCurrencies := currencySet(developedEmitCurrencyOrder)
	pppCurrencies := currencySet(pppEmitCurrencyOrder)

	developed := make(map[Plan]map[Period]map[string]Amount, len(planOrder))
	ppp := make(map[pppKey]Amount, expectedPPPRows)

	for i, r := range rows {
		plan, ok := knownPlan(r.Plan)
		if !ok {
			return nil, nil, fmt.Errorf("row %d (%s/%s/%s/%s): unknown plan %q", i, r.Plan, r.Period, r.Tier, r.Currency, r.Plan)
		}
		period, ok := knownPeriod(r.Period)
		if !ok {
			return nil, nil, fmt.Errorf("row %d (%s/%s/%s/%s): unknown period %q", i, r.Plan, r.Period, r.Tier, r.Currency, r.Period)
		}
		currency := strings.ToLower(r.Currency)
		amount := Amount{
			Currency:        currency,
			UnitAmountMinor: r.UnitAmountMinor,
			TaxBehavior:     canonicalTaxBehavior(r.TaxBehavior),
		}

		switch Tier(r.Tier) {
		case TierDeveloped:
			// An unknown currency is refused rather than dropped:
			// developedEmitCurrencyOrder is hand-maintained rendering order,
			// so a currency the console added but this package has never
			// heard of would otherwise vanish from the fallback silently.
			if !developedCurrencies[currency] {
				return nil, nil, fmt.Errorf("row %d (%s/%s/developed): currency %q is not in the developed emit order", i, r.Plan, r.Period, currency)
			}
			byPeriod, ok := developed[plan]
			if !ok {
				byPeriod = make(map[Period]map[string]Amount, len(periodEmitOrder))
				developed[plan] = byPeriod
			}
			byCurrency, ok := byPeriod[period]
			if !ok {
				byCurrency = make(map[string]Amount, len(developedEmitCurrencyOrder))
				byPeriod[period] = byCurrency
			}
			if _, dup := byCurrency[currency]; dup {
				return nil, nil, fmt.Errorf("row %d: duplicate developed row for %s/%s/%s", i, plan, period, currency)
			}
			byCurrency[currency] = amount
		case TierPPP:
			if !pppCurrencies[currency] {
				return nil, nil, fmt.Errorf("row %d (%s/%s/ppp): currency %q is not in the PPP emit order", i, r.Plan, r.Period, currency)
			}
			k := pppKey{plan: plan, period: period, currency: currency}
			if _, dup := ppp[k]; dup {
				return nil, nil, fmt.Errorf("row %d: duplicate PPP row for %s/%s/%s", i, plan, period, currency)
			}
			ppp[k] = amount
		default:
			return nil, nil, fmt.Errorf("row %d (%s/%s/%s): unknown tier %q", i, r.Plan, r.Period, r.Currency, r.Tier)
		}
	}

	if err := checkComplete(developed, ppp); err != nil {
		return nil, nil, err
	}
	return developed, ppp, nil
}

// checkComplete reports the first gap in either table, naming it, so a partial
// answer from the console fails generation loudly instead of producing a
// smaller catalog that still compiles.
func checkComplete(developed map[Plan]map[Period]map[string]Amount, ppp map[pppKey]Amount) error {
	var missing []string
	for _, plan := range planOrder {
		for _, period := range periodEmitOrder {
			for _, currency := range developedEmitCurrencyOrder {
				if _, ok := developed[plan][period][currency]; !ok {
					missing = append(missing, fmt.Sprintf("developed/%s/%s/%s", plan, period, currency))
				}
			}
			for _, currency := range pppEmitCurrencyOrder {
				if _, ok := ppp[pppKey{plan: plan, period: period, currency: currency}]; !ok {
					missing = append(missing, fmt.Sprintf("ppp/%s/%s/%s", plan, period, currency))
				}
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	const shown = 5
	preview, ellipsis := missing, ""
	if len(preview) > shown {
		preview, ellipsis = preview[:shown], ", …"
	}
	return fmt.Errorf("incomplete catalog: %d of %d amounts missing (%s%s) — "+
		"refusing to generate, because a fallback missing prices is worse than no change at all",
		len(missing), expectedDevelopedRows+expectedPPPRows,
		strings.Join(preview, ", "), ellipsis)
}

func currencySet(order []string) map[string]bool {
	set := make(map[string]bool, len(order))
	for _, c := range order {
		set[c] = true
	}
	return set
}

func knownPlan(v string) (Plan, bool) {
	for _, p := range planOrder {
		if Plan(v) == p {
			return p, true
		}
	}
	return "", false
}

func knownPeriod(v string) (Period, bool) {
	for _, p := range periodEmitOrder {
		if Period(v) == p {
			return p, true
		}
	}
	return "", false
}

// canonicalTaxBehavior maps the console's "unspecified" onto the empty string
// this catalog stores for Stripe's default. Confirmed against the live
// endpoint by consolecatalog's comparison, which had to make the same
// equivalence to reach a zero difference count.
func canonicalTaxBehavior(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "unspecified" {
		return ""
	}
	return v
}
