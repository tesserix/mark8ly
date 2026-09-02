// Package ssrfguard decides whether a merchant-supplied URL is safe for the
// cluster to dial.
//
// Webhooks are the first feature where merchant input becomes an egress
// target: every other outbound integration here (payment gateways, carriers,
// email providers) talks to fixed, configured endpoints. A merchant who can
// make us POST to an arbitrary URL can otherwise reach the GCP metadata
// server or any in-cluster service and read the response back out of the
// delivery log.
//
// Check is called at registration AND again immediately before every
// delivery. Registration-only validation is the usual shortcut and it is
// defeated by DNS rebinding — a hostname that answers public when saved and
// private when dialled.
package ssrfguard

import (
	"errors"
	"net"
	"net/url"
)

var (
	ErrNotHTTPS       = errors.New("webhook url must use https")
	ErrPrivateAddress = errors.New("webhook url resolves to a non-public address")
	ErrUnresolvable   = errors.New("webhook url host could not be resolved")
	ErrMalformed      = errors.New("webhook url is malformed")
	ErrTooLong        = errors.New("webhook url is too long")
)

// maxURLLen bounds what we will store and log.
const maxURLLen = 2048

// nonPublicRanges are blocks that net.IP's built-in classifiers do not
// cover but that must still be refused. Parsed once at package init rather
// than per-call.
var nonPublicRanges = mustParseCIDRs(
	// 0.0.0.0/8: IsUnspecified() only matches the all-zero address, but
	// Linux — the deployment target, Alpine containers — routinely treats
	// the whole "this network" block as loopback/local. A documented SSRF
	// bypass class.
	"0.0.0.0/8",
	// 100.64.0.0/10 (RFC 6598, carrier-grade NAT / shared address space):
	// IsPrivate() covers RFC1918 and fc00::/7 only, but this range is
	// exactly the kind of address cloud/Kubernetes internal networking
	// (CNI overlays, NAT gateways) uses on this cluster.
	"100.64.0.0/10",
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("ssrfguard: invalid CIDR literal " + c + ": " + err.Error())
		}
		nets = append(nets, n)
	}
	return nets
}

// Resolver looks a hostname up. Injected so tests need no DNS.
type Resolver func(host string) ([]net.IP, error)

type Guard struct{ resolve Resolver }

// New builds a Guard. Pass nil to use real DNS.
func New(r Resolver) *Guard {
	if r == nil {
		r = net.LookupIP
	}
	return &Guard{resolve: r}
}

// Check parses raw, requires https, resolves the host, and rejects the URL
// if ANY resolved address is non-public — an attacker needs only one private
// answer to be selected at dial time.
func (g *Guard) Check(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, ErrMalformed
	}
	if len(raw) > maxURLLen {
		return nil, ErrTooLong
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, ErrMalformed
	}
	if u.Scheme != "https" {
		return nil, ErrNotHTTPS
	}
	if u.Hostname() == "" {
		return nil, ErrMalformed
	}

	ips, err := g.resolve(u.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, ErrUnresolvable
	}
	for _, ip := range ips {
		if !isPublic(ip) {
			return nil, ErrPrivateAddress
		}
	}
	return u, nil
}

// isPublic reports whether ip is globally routable. Everything else —
// loopback, private, link-local (which covers 169.254.169.254, the cloud
// metadata endpoint), multicast, unspecified — is refused.
func isPublic(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, n := range nonPublicRanges {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
