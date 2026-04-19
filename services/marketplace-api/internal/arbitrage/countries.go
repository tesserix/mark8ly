// Package arbitrage encodes the geo-pricing triangulation check per spec §18.8.
// This file contains only the developed-vs-emerging market classifier —
// zero I/O, safe to call in a hot path.
package arbitrage

import (
	"strings"
	"unicode"
)

// developedMarkets is the exhaustive list that anti-arbitrage treats as
// "standard-tier eligible". Countries NOT in this set are either PPP-eligible
// or require a judgment call routed to billing-ops (§4.1.3 support escalation).
//
// The list aligns with the 13 billing currencies in §4.2.1 minus emerging
// markets. It is deliberately small — over-including countries risks flagging
// legitimate merchants; under-including risks missing arbitrage attempts. When
// the spec adds a currency, this list must be audited in the same PR.
var developedMarkets = map[string]struct{}{
	"US": {}, "CA": {}, "GB": {}, "IE": {},
	"DE": {}, "FR": {}, "IT": {}, "ES": {}, "NL": {},
	"AU": {}, "NZ": {}, "JP": {}, "SG": {},
}

// IsDevelopedMarket returns true for ISO-3166-1 alpha-2 codes in the developed
// set. Unknown, empty, or sentinel "??" input returns false — on ambiguity
// we err on the side of NOT flagging.
func IsDevelopedMarket(code string) bool {
	_, ok := developedMarkets[NormalizeCountry(code)]
	return ok
}

// IsKnownCountry returns true if the code is a valid 2-letter alpha code.
// Used by the appeal handler to reject obviously invalid jurisdiction claims.
// "??" is rejected (it's our internal sentinel for missing signal).
func IsKnownCountry(code string) bool {
	n := NormalizeCountry(code)
	return n != "??"
}

// NormalizeCountry returns an uppercased 2-letter code, or "??" when input is
// missing/invalid. Callers should propagate "??" into the audit row so the
// billing-ops reviewer can see which signal was missing.
func NormalizeCountry(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	if len(c) != 2 {
		return "??"
	}
	for _, r := range c {
		if !unicode.IsLetter(r) {
			return "??"
		}
	}
	return c
}
