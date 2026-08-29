// Package consolecatalog reads the plan catalog from the Tesserix console
// and compares it against the catalog compiled into
// internal/billing/pricing (#304, #392).
//
// # Why this exists
//
// The console is becoming the single place a price is maintained. Until
// marketplace-api reads from it, a price changed in the console updates
// Stripe while this service keeps serving the hardcoded Go amount — a
// divergence on the SERVING side that the console's parity check is
// structurally blind to, because that check compares the console's catalog
// to Stripe and both of those would be correct.
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
// # This package changes no behaviour yet
//
// Slice one runs both sources and logs their differences. Prices continue
// to come from the compiled catalog. The cutover happens separately, once
// the difference count is durably zero — the same evidence pattern the
// console's parity check established.
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
