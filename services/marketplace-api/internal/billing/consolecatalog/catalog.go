// Package consolecatalog reads the plan catalog from the Tesserix console
// and compares it against the catalog compiled into
// internal/billing/pricing (#304, #392).
//
// # Why this exists
//
// The console is the single place a price is maintained. Before
// marketplace-api read from it, a price changed in the console updated
// Stripe while this service kept serving the hardcoded Go amount — a
// divergence on the SERVING side that the console's parity check is
// structurally blind to, because that check compares the console's catalog
// to Stripe and both of those would be correct. Closing that divergence is
// what this package was built for, and #632 closed it.
//
// # The invariant that shapes every decision here
//
// BACKLOG §P: nothing on the request path of a customer payment may depend
// on the console being reachable. So the read is cached with a generous TTL
// (this data changes a few times a year), fails open to last-known, and
// falls back to the compiled catalog on a cold start — a fresh pod during a
// console outage must not itself be the outage. A cache that fails open is
// not a nicety; it is the entire reason reading the console at all is
// acceptable.
//
// # The cutover has happened — this package is on the serving path
//
// This header used to read "this package changes no behaviour yet", which
// was true of slice one and false from #632 onward. Since then the platform
// console's billing read routes resolve their amounts through Cache.Resolve
// here (see cmd/marketplace-api/catalog_serving.go), so a fault in this
// package is a fault in what an operator is shown, not just in a log line.
//
// The compiled catalog in internal/billing/pricing did not go away: it is
// the fallback Resolve serves from when the console is unconfigured or
// unreachable, and it is now GENERATED from the console by cmd/gencatalog
// (#648) rather than hand-maintained. The Monitor's comparison therefore
// answers a narrower question than it once did — no longer "did someone
// forget to update the Go map", but "did the console move without the
// fallback being regenerated". A difference is reported to Prometheus as
// well as logged (#328 gap 3), because a difference nothing watches is a
// difference nobody acts on.
package consolecatalog

import "time"

// Catalog is the console's published response. The shape is a contract
// owned by the console (tesserix-home#427): additive changes only, and a
// rename or type change belongs behind /api/v2.
type Catalog struct {
	Mode        string    `json:"mode"`
	RevisionID  string    `json:"revision_id"`
	PublishedAt time.Time `json:"published_at"`
	Prices      []Price   `json:"prices"`
}

// Price is one (lookup_key, currency) amount.
//
// Note the console returns one row PER CURRENCY, while the compiled catalog
// models a developed-tier price as one descriptor carrying an Options map of
// seven currencies. 42 lookup keys become 78 rows. Diff flattens both sides
// to the same row shape so the two are comparable at all.
type Price struct {
	LookupKey       string `json:"lookup_key"`
	Plan            string `json:"plan"`
	Period          string `json:"period"`
	Tier            string `json:"tier"`
	Currency        string `json:"currency"`
	UnitAmountMinor int64  `json:"unit_amount_minor"`
	TaxBehavior     string `json:"tax_behavior"`
}
