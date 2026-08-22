package domainvalidate

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type stubResolver struct {
	ns  []*net.NS
	err error
}

func (s stubResolver) LookupNS(_ context.Context, _ string) ([]*net.NS, error) {
	return s.ns, s.err
}

func TestSyntax_Rejections(t *testing.T) {
	cases := map[string]string{
		"":                       "required",
		"   ":                    "required",
		"localhost":              "full domain",
		"https://example.com":    "without https",
		"example.com/path":       "without https",
		"shop example.com":       "spaces",
		"127.0.0.1":              "IP address",
		"shop..example.com":      "valid domain label",
		"-example.com":           "valid domain label",
		"example-.com":           "valid domain label",
		strings.Repeat("a", 254): "longer than 253",
	}
	for in, want := range cases {
		_, err := checkSyntax(in)
		if err == nil {
			t.Errorf("%q: expected error", in)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %q does not contain %q", in, err.Error(), want)
		}
	}
}

func TestSyntax_HappyPath(t *testing.T) {
	cases := map[string]string{
		"example.com":          "example.com",
		"  Shop.Example.Com  ": "shop.example.com",
		"shop.example.com.":    "shop.example.com",
		"deep.sub.example.com": "deep.sub.example.com",
		"AbCdEf.example.co.uk": "abcdef.example.co.uk",
	}
	for in, want := range cases {
		got, err := checkSyntax(in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestCheck_NXDomainRejects(t *testing.T) {
	r := stubResolver{err: &net.DNSError{IsNotFound: true, Name: "totallyfake.example"}}
	_, err := Check(context.Background(), "totallyfake.example", r)
	if err == nil {
		t.Fatal("expected error for NXDOMAIN")
	}
	if !errors.Is(err, ErrInvalidDomain) {
		t.Fatalf("want ErrInvalidDomain, got %v", err)
	}
	if !strings.Contains(err.Error(), "registered") {
		t.Fatalf("expected 'registered' hint in error, got %q", err)
	}
}

func TestCheck_EmptyNSRejects(t *testing.T) {
	r := stubResolver{ns: nil}
	_, err := Check(context.Background(), "expired.example.com", r)
	if err == nil {
		t.Fatal("expected error for empty NS")
	}
}

func TestCheck_TransientFailsOpen(t *testing.T) {
	// Timeouts and "is temporary" classes should NOT block the user — the
	// later CNAME-verify flow has the final say. This protects merchants
	// from a flaky upstream resolver.
	r := stubResolver{err: &net.DNSError{IsTimeout: true}}
	got, err := Check(context.Background(), "shop.example.com", r)
	if err != nil {
		t.Fatalf("want fail-open on timeout, got %v", err)
	}
	if got != "shop.example.com" {
		t.Fatalf("canonical mismatch: %s", got)
	}
}

func TestCheck_Happy(t *testing.T) {
	r := stubResolver{ns: []*net.NS{{Host: "ns1.example.com."}}}
	got, err := Check(context.Background(), "Shop.Example.com", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "shop.example.com" {
		t.Fatalf("canonical: %s", got)
	}
}
