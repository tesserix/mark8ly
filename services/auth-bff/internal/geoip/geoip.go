// Package geoip resolves the country a request originated from.
//
// All mark8ly.com traffic reaches the cluster through a Cloudflare
// Tunnel, so the edge has already done the IP→country lookup and
// hands it to us in CF-IPCountry. That avoids shipping a MaxMind
// database and keeps the login path free of a network lookup.
package geoip

import (
	"net/http"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

const (
	headerCloudflare = "CF-IPCountry"
	headerFallback   = "X-Geo-Country"
)

// sentinels are the non-country values Cloudflare returns when it
// cannot place the client: XX for unknown, T1 for Tor exit nodes.
var sentinels = map[string]bool{"XX": true, "T1": true}

var regionNamer = display.Regions(language.English)

// CountryFromHeaders returns an ISO-3166-1 alpha-2 country code, or
// an empty string when the origin cannot be determined. Callers must
// treat "" as unknown rather than as a country.
func CountryFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	if c := normalise(h.Get(headerCloudflare)); c != "" {
		return c
	}
	return normalise(h.Get(headerFallback))
}

// normalise upper-cases and validates a candidate code, rejecting
// anything that is not two ASCII letters or is a known sentinel.
func normalise(raw string) string {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if len(c) != 2 || sentinels[c] {
		return ""
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return c
}

// Describe renders a country code for humans, e.g. "IN" → "India".
// Codes CLDR cannot name are returned verbatim — showing the raw code
// beats showing the literal phrase "Unknown Region" in a security email.
func Describe(code string) string {
	if code == "" {
		return "an unknown location"
	}
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	name := regionNamer.Name(region)
	if name == "" || name == "Unknown Region" {
		return code
	}
	return name
}
