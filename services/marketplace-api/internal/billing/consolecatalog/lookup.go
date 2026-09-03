package consolecatalog

import "strings"

// priceKey identifies one row of the flattened catalog.
//
// Four fields, not the lookup_key, because that is the vocabulary a caller
// actually holds: a subscription row carries plan, period, price_tier and
// billing_currency, and never a Stripe lookup_key. Diff keys on
// (lookup_key, currency) instead, which is right for ITS job — comparing two
// catalogs row for row — and useless for answering "what does this merchant
// pay".
//
// Every field is normalised to lower case here. The console's own schema
// already constrains three of them (apps/web/db/migrations/0032_plan_catalog.sql:
// period CHECK IN ('monthly','annual'), tier CHECK IN ('developed','ppp'),
// currency CHECK lowercase ISO 4217), so normalising is belt and braces on
// that side — but the CALLER's currency comes from a char(3) database column
// of unspecified case, and matching it against a lowercase catalog key
// without folding is how a real merchant's amount silently disappears from
// the console.
type priceKey struct {
	plan     string
	period   string
	tier     string
	currency string
}

func newPriceKey(plan, period, tier, currency string) priceKey {
	return priceKey{
		plan:     fold(plan),
		period:   fold(period),
		tier:     fold(tier),
		currency: fold(currency),
	}
}

func fold(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// PriceIndex answers "the price for (plan, period, tier, currency)" over one
// catalog.
//
// It is built once and queried many times because that is how the callers
// use it: a console page renders up to 500 subscription rows, and a linear
// scan of 78 prices per row is work nobody needs to do. Building it is also
// the point at which unusable rows are dropped once, rather than being
// re-examined at every lookup — see Index.
type PriceIndex struct {
	byKey map[priceKey]Price
}

// Index builds a PriceIndex over the catalog's prices.
//
// Rows with an empty currency are SKIPPED rather than indexed. That is the
// same rule compiledRows and CompiledCatalog apply, for the same reason: a
// row that names no currency names no price, and indexing it would let a
// lookup return an amount of zero for a currency the catalog has nothing to
// say about. The console cannot produce such a row (plan_catalog_amounts
// constrains currency to lowercase ISO 4217), but CompiledCatalog projects
// from a map that IS pre-populated with zero-value entries, so the guard has
// to live on this side of the boundary too.
//
// A duplicate key keeps the FIRST row. Neither source is expected to produce
// one — the console's schema is UNIQUE (revision_id, source, lookup_key) with
// one amount per currency — so this only decides what happens if that ever
// stops being true, and "keep what you first saw" is at least stable across
// calls where last-write-wins over a slice would not be.
func (c Catalog) Index() PriceIndex {
	ix := PriceIndex{byKey: make(map[priceKey]Price, len(c.Prices))}
	for _, p := range c.Prices {
		if fold(p.Currency) == "" {
			continue
		}
		k := newPriceKey(p.Plan, p.Period, p.Tier, p.Currency)
		if _, dup := ix.byKey[k]; dup {
			continue
		}
		ix.byKey[k] = p
	}
	return ix
}

// Len reports how many usable prices the index holds. Zero means the index
// can price nothing at all, which a caller must be able to tell apart from
// "this particular combination is unpriced".
func (ix PriceIndex) Len() int { return len(ix.byKey) }

// Find returns the price for one (plan, period, tier, currency), matching
// case-insensitively on all four.
//
// ok=false means the catalog has no such row, and the returned Price is then
// the zero value — an amount of 0 in no currency, which a caller must not
// report as a price. There is deliberately no way to get ok=true out of this
// without a row that was actually indexed: absence is reported by the bool,
// never by a zero amount. That property is the whole reason Find returns
// (Price, bool) rather than an amount, and the reason Index drops rows with
// an empty currency instead of storing them.
func (ix PriceIndex) Find(plan, period, tier, currency string) (Price, bool) {
	p, ok := ix.byKey[newPriceKey(plan, period, tier, currency)]
	if !ok {
		return Price{}, false
	}
	return p, true
}
