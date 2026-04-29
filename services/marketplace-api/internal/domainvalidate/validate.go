// Package domainvalidate runs the syntactic + existence checks the
// custom-domain UI does before letting a merchant submit. The same
// checks run inside the Add path as defense-in-depth — any caller
// that bypasses the UI still hits the same gate.
//
// Checks performed (in order, fail fast on the first):
//
//  1. Syntactic — RFC 1034/1123: each label is 1..63 chars,
//     letters / digits / hyphens, must not start or end with '-'.
//     Total length ≤ 253. At least one dot, no IP literal.
//
//  2. Public-suffix sanity — at least an apex + TLD. We do NOT block
//     specific TLDs.
//
//  3. Authoritative NS lookup — a registered domain has at least one
//     NS record published by its registrar's nameservers. Unregistered
//     or expired domains return NXDOMAIN or zero NS records.
//
// What this is NOT: a check that the merchant *owns* the domain. That
// happens later via the CNAME / acme-challenge verify step. This pass
// only ensures the domain is real — i.e. a typo'd or fictitious name
// is caught at form-submit time.
package domainvalidate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// ErrInvalidDomain is the sentinel for any failure produced by Check.
// Callers map it to a 422-style validation error; the wrapped string
// is safe to surface verbatim to the merchant.
var ErrInvalidDomain = errors.New("invalid domain")

// labelRE is the per-label pattern. RFC 1123 relaxes the strict 1034
// rule that labels must start with a letter; we accept the common
// modern form (letter/digit/hyphen, no leading or trailing hyphen).
var labelRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// Resolver is the slice of net.Resolver we depend on. Tests substitute
// a stub; production passes net.DefaultResolver.
type Resolver interface {
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
}

// Check runs the full validation pipeline. resolver may be nil — the
// system default is used.
func Check(ctx context.Context, raw string, resolver Resolver) (canonical string, err error) {
	canonical, err = checkSyntax(raw)
	if err != nil {
		return "", err
	}
	// Hard cap the DNS round-trip so a slow/blackholed nameserver
	// can't lock up the request thread on the admin path.
	dnsCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := checkExistence(dnsCtx, canonical, resolver); err != nil {
		return "", err
	}
	return canonical, nil
}

// checkSyntax normalises and validates the domain shape. Returns the
// canonical lower-cased FQDN with no trailing dot.
func checkSyntax(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(s)

	if s == "" {
		return "", fmt.Errorf("%w: domain is required", ErrInvalidDomain)
	}
	if len(s) > 253 {
		return "", fmt.Errorf("%w: domain is longer than 253 characters", ErrInvalidDomain)
	}
	if strings.Contains(s, "://") || strings.Contains(s, "/") {
		return "", fmt.Errorf("%w: enter the domain only, without https:// or paths", ErrInvalidDomain)
	}
	if strings.Contains(s, " ") {
		return "", fmt.Errorf("%w: domain cannot contain spaces", ErrInvalidDomain)
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return "", fmt.Errorf("%w: enter a domain name, not an IP address", ErrInvalidDomain)
	}

	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("%w: enter a full domain like example.com", ErrInvalidDomain)
	}
	for _, l := range labels {
		if !labelRE.MatchString(l) {
			return "", fmt.Errorf("%w: %q is not a valid domain label", ErrInvalidDomain, l)
		}
	}
	tld := labels[len(labels)-1]
	if len(tld) < 2 {
		return "", fmt.Errorf("%w: domain must end in a valid TLD", ErrInvalidDomain)
	}
	return s, nil
}

// checkExistence verifies the registrable apex has authoritative
// nameservers. We deliberately query NS on the apex — not on the
// merchant's full FQDN — because:
//
//   - The apex is what the registrar publishes. A subdomain may not
//     have its own NS RRset and that's not a sign of "doesn't exist".
//   - "shop.example.com" should still validate as long as
//     "example.com" is registered and active.
//
// On NXDOMAIN or empty NS we surface a friendly "domain doesn't seem
// to be registered" message. On a non-DNS error (timeout, network
// blip) we fail open — the worst case is the user proceeds and the
// later CNAME verification flow catches it, which is preferable to
// blocking a real merchant on a transient resolver outage.
func checkExistence(ctx context.Context, fqdn string, resolver Resolver) error {
	apex, err := apexOf(fqdn)
	if err != nil {
		return err
	}
	ns, lookupErr := resolver.LookupNS(ctx, apex)
	if lookupErr != nil {
		var dnsErr *net.DNSError
		if errors.As(lookupErr, &dnsErr) {
			if dnsErr.IsNotFound {
				return fmt.Errorf(
					"%w: %s doesn't appear to be a registered domain. Check the spelling, or register it first.",
					ErrInvalidDomain, apex,
				)
			}
			if dnsErr.IsTemporary || dnsErr.IsTimeout {
				// Fail open on transient errors — caller proceeds,
				// later verify flow has the final say.
				return nil
			}
		}
		// Unknown error class — treat as transient.
		return nil
	}
	if len(ns) == 0 {
		return fmt.Errorf(
			"%w: %s has no nameservers — it's likely expired or not yet registered.",
			ErrInvalidDomain, apex,
		)
	}
	return nil
}

// apexOf returns the registrable apex for an FQDN. Same caveat as
// cfclient.apexOf: a single-segment TLD heuristic. Multi-segment
// public suffixes (e.g. co.uk) would over-strip — we accept that
// trade-off here because the worst case is an extra-strict NS check
// against e.g. "company.co.uk" (which still has NS records). A real
// PSL dependency is overkill for this validation path.
func apexOf(fqdn string) (string, error) {
	parts := strings.Split(fqdn, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("%w: missing TLD", ErrInvalidDomain)
	}
	return strings.Join(parts[len(parts)-2:], "."), nil
}
