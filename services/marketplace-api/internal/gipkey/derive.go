// Package gipkey keeps the Google Identity Platform browser API key's
// HTTP-referrer allowlist in sync with the merchant custom domains that
// the storefront serves. When a custom domain is verified, the
// storefront's browser-side signInWithPassword call would be rejected
// with API_KEY_HTTP_REFERRER_BLOCKED unless the domain (and its
// wildcard subdomain) is on the key's allowedReferrers list. This
// package gives the domain service a self-service hook that adds the
// patterns on verify and removes them on takedown.
package gipkey

import (
	"strings"
)

// DeriveReferrers returns the canonical pair of allowlist patterns for
// a custom domain — the apex and a single-level wildcard subdomain
// covering www. and any future <slug>.<apex> usage by the same
// merchant. The form mirrors the existing entries already on the
// gip-web-key-v2 key (e.g. "https://mark8ly.com/*",
// "https://*.mark8ly.com/*").
//
// Returns an empty slice when the input is not a usable FQDN so
// callers don't push junk patterns to GCP and trigger a 400 on update.
func DeriveReferrers(domain string) []string {
	fqdn := strings.ToLower(strings.TrimSpace(domain))
	fqdn = strings.TrimSuffix(fqdn, ".")
	if fqdn == "" {
		return nil
	}
	// Require at least one dot — single-label hostnames aren't valid
	// browser key referrers and almost always indicate a bug upstream.
	if !strings.Contains(fqdn, ".") {
		return nil
	}
	// Reject anything that looks like a URL — callers should pass a
	// bare FQDN. Defending here keeps a stray "https://example.com"
	// out of the allowlist.
	if strings.ContainsAny(fqdn, "/:?#") {
		return nil
	}
	return []string{
		"https://" + fqdn + "/*",
		"https://*." + fqdn + "/*",
	}
}
