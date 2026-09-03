// Package zitadellogin: return-URL allowlisting.
//
// Starting a Zitadel IDP intent means handing Zitadel a successUrl. After
// the user authenticates with the upstream IDP (Google), the browser is
// redirected there carrying an intent id and token that can be exchanged for
// a completed federated identity. Zitadel does not validate that URL at all
// — an intent with successUrl set to an attacker-controlled origin is
// accepted and returns a working authUrl. This file is therefore the ONLY
// thing standing between /auth/*/idp/start and an open redirect that hands a
// finished sign-in to an attacker's domain.
//
// The allowlist is deliberately host-shaped, not pattern-shaped: an exact
// set of hosts (the fixed admin host, plus anything else that should match
// verbatim) and a set of dot-prefixed suffixes for the per-tenant storefront
// subdomains (e.g. ".mark8ly.com" permits shop.mark8ly.com without also
// permitting the bare mark8ly.com or evil-mark8ly.com).
package zitadellogin

import (
	"errors"
	"net/url"
	"strings"
)

// ErrReturnURLNotAllowed is returned when a candidate return URL fails the
// allowlist check for any reason. Callers must not construct behaviour off
// the specific reason string — treat this as a boolean decision.
var ErrReturnURLNotAllowed = errors.New("zitadellogin: return url not allowed")

// ReturnURLAllowlist is the set of hosts a Zitadel IDP-intent successUrl or
// failureUrl is permitted to point at.
//
// Hosts must match a request host exactly (case-insensitively). Suffixes
// permit any subdomain of the given domain (also case-insensitively) but
// NOT the bare domain itself — list it in Hosts too if the bare domain
// should also be permitted.
//
// Construct with NewReturnURLAllowlist rather than building the struct
// directly, so entries are normalized once instead of on every check.
type ReturnURLAllowlist struct {
	hosts    map[string]struct{}
	suffixes []string
}

// NewReturnURLAllowlist builds an allowlist from configured hosts and
// suffixes. suffixes entries are normalized to always start with a leading
// dot (a bare "mark8ly.com" is treated the same as ".mark8ly.com") so a
// misconfigured env var without the dot does not silently turn into a
// substring match.
func NewReturnURLAllowlist(hosts, suffixes []string) ReturnURLAllowlist {
	a := ReturnURLAllowlist{hosts: make(map[string]struct{}, len(hosts))}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		a.hosts[h] = struct{}{}
	}
	for _, s := range suffixes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		a.suffixes = append(a.suffixes, s)
	}
	return a
}

// allowsHost reports whether host (already lower-cased) is permitted.
func (a ReturnURLAllowlist) allowsHost(host string) bool {
	if _, ok := a.hosts[host]; ok {
		return true
	}
	for _, suffix := range a.suffixes {
		// strings.HasSuffix on the parsed, lower-cased host — not a
		// substring/prefix test on the raw URL — so
		// "mark8ly.com.evil.tld" (which merely CONTAINS an allowed
		// host) and "evilmark8ly.com" (which shares a suffix of
		// characters but not a label boundary) are both rejected: the
		// suffix must be preceded by the label separator itself.
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// isHostnameChar reports whether r is a character legitimate DNS hostnames
// use. Rejecting anything else after parsing is defence in depth against
// percent-encoding, raw unicode, or other homograph/normalization tricks
// riding through in the host component: a host allowed by this check still
// has to equal (or share a label-bounded suffix with) a literal ASCII entry
// in the allowlist to pass, but this stops confusable-but-parseable hosts
// from ever reaching that comparison in a form that could be misread.
func isHostnameChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-' || r == '.':
		return true
	}
	return false
}

// ValidateReturnURL checks candidate against the allowlist and returns it
// unchanged if permitted. It never echoes candidate back in the returned
// error — the value is attacker-controlled input, and a caller that logs the
// error verbatim must not be exposed to log injection via it.
//
// Deliberate decisions, so a reviewer does not have to re-derive them:
//
//   - https only. Plain http://, scheme-relative ("//host/..."), and
//     schemeless input are all rejected (scheme-relative and schemeless
//     parse to an empty Scheme, which fails the exact "https" check).
//   - Userinfo (https://user:pass@host/...) is rejected outright, even
//     when the host itself is on the allowlist. It serves no legitimate
//     purpose on a login redirect and is a classic phishing/confusion
//     vector, so it is refused rather than merely ignored.
//   - An explicit port is rejected. Production return URLs are plain
//     https on the default port; a port is either a mistake or an
//     attempt to reach something unexpected on an otherwise-allowed host.
//   - Host comparison is case-insensitive (DNS is case-insensitive) but
//     otherwise exact: no trailing-dot normalization. A trailing dot
//     ("mark8ly.com.") therefore fails to match any allowlist entry and
//     is rejected, rather than being silently equated with the
//     non-dotted form.
//   - IPv6 literals and bare IPs are never in the allowlist by
//     construction, so they are rejected as a natural consequence of the
//     host comparison, not through separate IP-specific logic.
//   - Empty and unparseable input is rejected before any host logic
//     runs.
func (a ReturnURLAllowlist) ValidateReturnURL(candidate string) (string, error) {
	if candidate == "" {
		return "", ErrReturnURLNotAllowed
	}
	u, err := url.Parse(candidate)
	if err != nil {
		return "", ErrReturnURLNotAllowed
	}
	if u.Scheme != "https" {
		return "", ErrReturnURLNotAllowed
	}
	if u.User != nil {
		return "", ErrReturnURLNotAllowed
	}
	if u.Port() != "" {
		return "", ErrReturnURLNotAllowed
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", ErrReturnURLNotAllowed
	}
	for _, r := range host {
		if !isHostnameChar(r) {
			return "", ErrReturnURLNotAllowed
		}
	}
	if !a.allowsHost(host) {
		return "", ErrReturnURLNotAllowed
	}
	return candidate, nil
}
