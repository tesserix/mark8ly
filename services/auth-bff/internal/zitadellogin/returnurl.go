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
// set of hosts (a fixed domain, matched verbatim) and a set of dot-prefixed
// suffixes for per-tenant subdomains (e.g. ".mark8ly.com" permits
// shop.mark8ly.com without also permitting the bare mark8ly.com or
// evil-mark8ly.com).
//
// This type is deliberately flow-agnostic: it knows nothing about "admin" or
// "storefront". A merchant-controlled storefront subdomain is a valid
// successUrl for a customer sign-in but must NEVER be valid for an admin
// sign-in — that boundary is expressed by constructing and selecting between
// TWO separate ReturnURLAllowlist values at the call site (see
// config.Config's *Admin/*Storefront fields), not by anything in this type.
package zitadellogin

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// ErrReturnURLNotAllowed is returned when a candidate return URL fails the
// allowlist check for any reason. Callers must not construct behaviour off
// the specific reason string — treat this as a boolean decision.
var ErrReturnURLNotAllowed = errors.New("zitadellogin: return url not allowed")

// ErrReturnURLAllowlistInvalid is returned by NewReturnURLAllowlist when a
// configured entry is not a safe, unambiguous host or suffix. This must fail
// at construction (boot), not at request time: a bad entry here — a bare
// TLD, a stray leading dot, an entry carrying a scheme or path — silently
// turns the only open-redirect control into a no-op while everything still
// LOOKS configured, and Zitadel itself does not validate successUrl at all.
// A wrong allowlist must stop the service, never quietly permit the
// internet.
var ErrReturnURLAllowlistInvalid = errors.New("zitadellogin: return url allowlist misconfigured")

// maxReturnURLLength bounds accepted candidates. Generous for any real
// redirect URL (query string included) while refusing to let an
// unboundedly-long value flow through the rest of the request pipeline.
const maxReturnURLLength = 2048

// ReturnURLAllowlist is the set of hosts a Zitadel IDP-intent successUrl or
// failureUrl is permitted to point at.
//
// Hosts must match a request host exactly (case-insensitively). Suffixes
// permit any subdomain of the given domain (also case-insensitively) but
// NOT the bare domain itself — list it in Hosts too if the bare domain
// should also be permitted.
//
// Construct with NewReturnURLAllowlist rather than building the struct
// directly, so entries are validated and normalized once instead of on
// every check.
type ReturnURLAllowlist struct {
	hosts    map[string]struct{}
	suffixes []string
}

// NewReturnURLAllowlist builds an allowlist from configured hosts and
// suffixes, or reports every entry that is not a safe, unambiguous domain.
// suffixes entries are normalized to always start with a leading dot (a bare
// "mark8ly.com" is treated the same as ".mark8ly.com") so a misconfigured
// env var without the dot does not silently turn into a substring match.
//
// Rejected outright (named in the returned error, since these are
// operator-authored config values, not attacker input, and the whole point
// of failing here is that the operator can see and fix the offending
// entry):
//   - empty, or a bare "." with nothing else
//   - fewer than two labels once any leading dot is stripped (a bare TLD
//     like "com" would make every ".com" domain a valid return host)
//   - any empty label, e.g. from a stray leading/trailing/doubled dot
//   - any of ':', '/', '@', '*', or whitespace — these indicate a scheme,
//     path, credential, or wildcard snuck into what must be a bare host
func NewReturnURLAllowlist(hosts, suffixes []string) (ReturnURLAllowlist, error) {
	a := ReturnURLAllowlist{hosts: make(map[string]struct{}, len(hosts))}
	var problems []string

	for _, raw := range hosts {
		h := strings.ToLower(strings.TrimSpace(raw))
		if h == "" {
			continue
		}
		if err := validateDomainEntry(h); err != nil {
			problems = append(problems, fmt.Sprintf("host %q: %v", raw, err))
			continue
		}
		a.hosts[h] = struct{}{}
	}

	for _, raw := range suffixes {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		domain := strings.TrimPrefix(s, ".")
		if err := validateDomainEntry(domain); err != nil {
			problems = append(problems, fmt.Sprintf("suffix %q: %v", raw, err))
			continue
		}
		a.suffixes = append(a.suffixes, "."+domain)
	}

	if len(problems) > 0 {
		return ReturnURLAllowlist{}, fmt.Errorf("%w: %s", ErrReturnURLAllowlistInvalid, strings.Join(problems, "; "))
	}
	return a, nil
}

// validateDomainEntry reports why domain (already lower-cased, with any
// leading dot already stripped by the caller) is not safe to use as an
// allowlist host or suffix.
func validateDomainEntry(domain string) error {
	if domain == "" {
		return errors.New("empty")
	}
	for _, r := range domain {
		switch {
		case r == ':' || r == '/' || r == '@' || r == '*':
			return fmt.Errorf("contains %q", r)
		case unicode.IsSpace(r):
			return errors.New("contains whitespace")
		}
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return errors.New("must have at least two labels (a bare TLD matches every domain under it)")
	}
	for _, l := range labels {
		if l == "" {
			return errors.New("has an empty label")
		}
	}
	return nil
}

// allowsHost reports whether host (already lower-cased, already checked for
// empty labels by the caller) is permitted.
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
		// suffix must be preceded by the label separator itself. This
		// also means the bare domain itself (host == "mark8ly.com"
		// against suffix ".mark8ly.com") does NOT match: the suffix
		// string is one character longer than the bare domain, so
		// HasSuffix is false for it — list the bare domain in Hosts
		// too if it should also be permitted.
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// isHostnameChar reports whether r is a character legitimate DNS hostnames
// use. Applied to the parsed request host, after url.Parse has already
// rejected the URL for most malformed input (including percent-encoded and
// control-character bytes in the host, which url.Parse itself refuses to
// decode into invalid UTF-8 or otherwise errors on) — so in practice this
// loop's remaining job is catching a syntactically valid but non-ASCII host
// (raw unicode/homograph characters), which url.Parse happily accepts.
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
// error verbatim must not be exposed to log injection via it. Callers must
// likewise not log the returned/accepted value verbatim on their own path;
// treat it as sensitive redirect data, not a safe-to-log identifier.
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
//   - A host with an empty label — a leading, trailing, or doubled dot
//     such as ".mark8ly.com" or "mark8ly..com" — is rejected outright.
//     Without this, ".mark8ly.com" trivially satisfies a HasSuffix check
//     against the allowlist suffix ".mark8ly.com" while carrying no real
//     subdomain at all.
//   - Input longer than maxReturnURLLength is rejected before parsing.
//   - Empty and unparseable input is rejected before any host logic
//     runs.
func (a ReturnURLAllowlist) ValidateReturnURL(candidate string) (string, error) {
	if candidate == "" || len(candidate) > maxReturnURLLength {
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
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return "", ErrReturnURLNotAllowed
		}
	}
	if !a.allowsHost(host) {
		return "", ErrReturnURLNotAllowed
	}
	return candidate, nil
}
