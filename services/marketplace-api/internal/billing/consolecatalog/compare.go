package consolecatalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// Difference is one disagreement between the two sources.
type Difference struct {
	LookupKey string
	Currency  string
	Detail    string
}

// rowKey identifies one amount on both sides.
type rowKey struct {
	lookupKey string
	currency  string
}

type row struct {
	amountMinor int64
	taxBehavior string
	// plan, period and tier are compared as well as keyed on, and that is
	// deliberate rather than redundant.
	//
	// `rowKey` is (lookup_key, currency) because that is what identifies an
	// amount on both sides. These three are the fields the SERVING lookup
	// keys on: `platformadmin`'s price index finds a row by (plan, period,
	// tier, currency), because a subscription row carries those and never a
	// Stripe lookup_key. Before they were compared here, `differences=0`
	// evidenced the amounts while saying nothing about the three fields the
	// cutover reads by -- so the check that gates the cutover did not cover
	// the path the cutover creates.
	//
	// Verified identical across all 42 lookup keys on 2026-09-03, so adding
	// them reports nothing new today. That is the point: the guard is being
	// put in place while it is known to pass, so a later divergence is a
	// signal rather than a discovery.
	plan   string
	period string
	tier   string
}

// Diff compares the console's catalog against the compiled one and returns
// every disagreement, sorted for stable logging.
//
// Its value is being trustworthy in both directions: a comparator that
// reports spurious differences trains everyone to ignore it, and one that
// misses real differences makes the cutover unsafe. The two normalisations
// below exist for the first reason and are load-bearing.
func Diff(console Catalog, compiled []pricing.PriceDescriptor) []Difference {
	left := consoleRows(console)
	right := compiledRows(compiled)

	var diffs []Difference
	for k, l := range left {
		r, ok := right[k]
		if !ok {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("present in the console (%d minor) but absent from the compiled catalog", l.amountMinor)})
			continue
		}
		if l.amountMinor != r.amountMinor {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("unit_amount_minor: compiled=%d console=%d", r.amountMinor, l.amountMinor)})
			continue
		}
		if !sameTaxBehavior(l.taxBehavior, r.taxBehavior) {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("tax_behavior: compiled=%q console=%q", r.taxBehavior, l.taxBehavior)})
			continue
		}
		// Reported one field at a time, worst-first like the checks above, so
		// a report names the specific field rather than dumping both rows and
		// leaving a reader to spot the difference.
		if l.plan != r.plan {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("plan: compiled=%q console=%q", r.plan, l.plan)})
			continue
		}
		if l.period != r.period {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("period: compiled=%q console=%q", r.period, l.period)})
			continue
		}
		if l.tier != r.tier {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("tier: compiled=%q console=%q", r.tier, l.tier)})
		}
	}
	for k, r := range right {
		if _, ok := left[k]; !ok {
			diffs = append(diffs, Difference{k.lookupKey, k.currency,
				fmt.Sprintf("present in the compiled catalog (%d minor) but absent from the console", r.amountMinor)})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].LookupKey != diffs[j].LookupKey {
			return diffs[i].LookupKey < diffs[j].LookupKey
		}
		return diffs[i].Currency < diffs[j].Currency
	})
	return diffs
}

func consoleRows(c Catalog) map[rowKey]row {
	out := make(map[rowKey]row, len(c.Prices))
	for _, p := range c.Prices {
		out[rowKey{p.LookupKey, strings.ToLower(p.Currency)}] = row{
			amountMinor: p.UnitAmountMinor,
			taxBehavior: p.TaxBehavior,
			// Lower-folded on both sides so a case difference alone cannot
			// report a divergence. The serving index folds the same way.
			plan:   strings.ToLower(strings.TrimSpace(p.Plan)),
			period: strings.ToLower(strings.TrimSpace(p.Period)),
			tier:   strings.ToLower(strings.TrimSpace(p.Tier)),
		}
	}
	return out
}

// compiledRows flattens descriptors to one row per currency.
//
// It walks Options rather than Baseline alone: a developed-tier descriptor
// is one Price object carrying seven currencies, and comparing only the
// baseline would leave six of every seven amounts unchecked.
//
// A zero-value Amount in Options is SKIPPED, not compared. catalog.go's init
// pre-populates the map with an entry for every developed currency even when
// no price exists for it, so map presence alone is not proof of a real price
// — the same trap money.go documents. Comparing those would report a
// permanent, uncloseable difference for every catalog gap.
func compiledRows(descriptors []pricing.PriceDescriptor) map[rowKey]row {
	out := make(map[rowKey]row)
	for _, d := range descriptors {
		for cur, amt := range d.Options {
			if amt.Currency == "" {
				continue
			}
			out[rowKey{d.LookupKey, strings.ToLower(cur)}] = row{
				amountMinor: amt.UnitAmountMinor,
				taxBehavior: amt.TaxBehavior,
				plan:        strings.ToLower(strings.TrimSpace(string(d.Plan))),
				period:      strings.ToLower(strings.TrimSpace(string(d.Period))),
				tier:        strings.ToLower(strings.TrimSpace(string(d.Tier))),
			}
		}
	}
	return out
}

// sameTaxBehavior treats the compiled catalog's empty string and the
// console's "unspecified" as the same fact.
//
// Both mean "Stripe's default". Compared naively, EVERY price in the catalog
// reads as divergent, the difference count never reaches zero, and the
// cutover this gates never happens. Confirmed against the live endpoint on
// 2026-08-30: console rows carry "unspecified" where catalog.go carries "".
func sameTaxBehavior(a, b string) bool {
	return normalizeTaxBehavior(a) == normalizeTaxBehavior(b)
}

func normalizeTaxBehavior(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "unspecified"
	}
	return v
}
