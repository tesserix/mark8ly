package ssrfguard

import (
	"net"
	"strings"
	"testing"
)

func fixed(ips ...string) Resolver {
	return func(string) ([]net.IP, error) {
		out := make([]net.IP, 0, len(ips))
		for _, s := range ips {
			out = append(out, net.ParseIP(s))
		}
		return out, nil
	}
}

func TestCheck_AllowsPublicHTTPS(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	u, err := g.Check("https://hooks.example.com/mark8ly")
	if err != nil {
		t.Fatalf("expected public https URL to pass, got %v", err)
	}
	if u.Host != "hooks.example.com" {
		t.Fatalf("host = %q", u.Host)
	}
}

func TestCheck_RejectsPlainHTTP(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	if _, err := g.Check("http://hooks.example.com/x"); err != ErrNotHTTPS {
		t.Fatalf("want ErrNotHTTPS, got %v", err)
	}
}

// Each of these is a documented SSRF target. A merchant-supplied URL that
// resolves to any of them must never be dialled.
func TestCheck_RejectsNonPublicDestinations(t *testing.T) {
	for name, ip := range map[string]string{
		"loopback":          "127.0.0.1",
		"private 10/8":      "10.0.0.5",
		"private 172.16/12": "172.16.0.5",
		"private 192.168":   "192.168.1.5",
		"link-local":        "169.254.1.1",
		"GCP metadata":      "169.254.169.254",
		"IPv6 loopback":     "::1",
		"IPv6 ULA":          "fd00::1",
		"unspecified":       "0.0.0.0",
	} {
		t.Run(name, func(t *testing.T) {
			g := New(fixed(ip))
			if _, err := g.Check("https://evil.example.com/x"); err != ErrPrivateAddress {
				t.Fatalf("want ErrPrivateAddress for %s, got %v", ip, err)
			}
		})
	}
}

// A hostname that resolves to a mix must be rejected — an attacker only
// needs one private answer to be picked at dial time.
func TestCheck_RejectsWhenAnyResolvedAddressIsPrivate(t *testing.T) {
	g := New(fixed("93.184.216.34", "127.0.0.1"))
	if _, err := g.Check("https://mixed.example.com/x"); err != ErrPrivateAddress {
		t.Fatalf("want ErrPrivateAddress, got %v", err)
	}
}

func TestCheck_RejectsUnresolvable(t *testing.T) {
	g := New(func(string) ([]net.IP, error) { return nil, net.UnknownNetworkError("nope") })
	if _, err := g.Check("https://nx.example.com/x"); err != ErrUnresolvable {
		t.Fatalf("want ErrUnresolvable, got %v", err)
	}
}

func TestCheck_RejectsGarbage(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	for _, raw := range []string{"", "not a url", "https://", "ftp://x.example.com"} {
		if _, err := g.Check(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestCheck_RejectsOverlongURL(t *testing.T) {
	g := New(fixed("93.184.216.34"))
	long := "https://hooks.example.com/" + strings.Repeat("a", 3000)
	if _, err := g.Check(long); err == nil {
		t.Fatal("expected an overlong URL to be rejected")
	}
}
