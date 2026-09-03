package zitadellogin

import (
	"errors"
	"strings"
	"testing"
)

func testAllowlist(t *testing.T) ReturnURLAllowlist {
	t.Helper()
	a, err := NewReturnURLAllowlist(
		[]string{"admin.mark8ly.com"},
		[]string{".mark8ly.com"},
	)
	if err != nil {
		t.Fatalf("NewReturnURLAllowlist: %v", err)
	}
	return a
}

func TestValidateReturnURL(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		wantOK    bool
	}{
		{"exact allowed host passes", "https://admin.mark8ly.com/callback", true},
		{"permitted tenant subdomain passes", "https://shop.mark8ly.com/callback", true},
		{"deeper tenant subdomain passes", "https://a.b.mark8ly.com/callback", true},

		{"http is rejected", "http://admin.mark8ly.com/callback", false},
		{"scheme-relative is rejected", "//admin.mark8ly.com/callback", false},
		{"schemeless is rejected", "admin.mark8ly.com/callback", false},

		{"host merely containing an allowed host is rejected", "https://mark8ly.com.evil.tld/callback", false},
		{"host sharing a character suffix but not a label boundary is rejected", "https://evilmark8ly.com/callback", false},
		{"bare apex of a suffix-only domain is rejected", "https://mark8ly.com/callback", false},
		{"unrelated host is rejected", "https://evil.example.com/callback", false},

		{"userinfo is rejected even on an allowed host", "https://user:pass@admin.mark8ly.com/callback", false},
		{"userinfo without password is rejected", "https://user@admin.mark8ly.com/callback", false},

		{"explicit port is rejected", "https://admin.mark8ly.com:8443/callback", false},

		{"trailing dot on host is rejected", "https://admin.mark8ly.com./callback", false},
		{"uppercase host is allowed (DNS is case-insensitive)", "https://ADMIN.MARK8LY.COM/callback", true},

		{"ipv6 literal host is rejected", "https://[::1]/callback", false},
		{"bare ip host is rejected", "https://93.184.216.34/callback", false},

		{"percent-encoded host trick is rejected", "https://admin.mark8ly.com%2eevil.tld/callback", false},
		{"unicode host trick is rejected", "https://admin.mark8ly.com․evil.tld/callback", false},

		{"empty input is rejected", "", false},
		{"unparseable input is rejected", "https://%zz", false},
		{"control character in host is rejected", "https://admin.mark8ly.com\n.evil.tld/callback", false},

		// Finding 4: an empty DNS label in the request host must not
		// trivially satisfy a suffix's HasSuffix check.
		{"leading dot producing an empty label is rejected", "https://.mark8ly.com/callback", false},
		{"doubled dot producing an empty label is rejected", "https://..mark8ly.com/callback", false},
		{"empty label mid-host is rejected", "https://shop..mark8ly.com/callback", false},

		// Finding 5: an absurdly long candidate is rejected outright,
		// even if it would otherwise resolve to an allowed host.
		{"overlong candidate is rejected", "https://admin.mark8ly.com/" + strings.Repeat("a", maxReturnURLLength), false},
	}

	a := testAllowlist(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.ValidateReturnURL(tc.candidate)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("ValidateReturnURL(%q) err = %v, want nil", tc.candidate, err)
				}
				if got != tc.candidate {
					t.Fatalf("ValidateReturnURL(%q) = %q, want unchanged", tc.candidate, got)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateReturnURL(%q) err = nil, want rejection", tc.candidate)
			}
			if !errors.Is(err, ErrReturnURLNotAllowed) {
				t.Fatalf("ValidateReturnURL(%q) err = %v, want ErrReturnURLNotAllowed", tc.candidate, err)
			}
		})
	}
}

func TestValidateReturnURLErrorDoesNotEchoTheCandidate(t *testing.T) {
	a := testAllowlist(t)
	candidate := "https://evil.tld/inject\nSESSION_TOKEN=leaked-looking-value"
	_, err := a.ValidateReturnURL(candidate)
	if err == nil {
		t.Fatal("expected rejection")
	}
	if got := err.Error(); got != ErrReturnURLNotAllowed.Error() {
		t.Fatalf("error text = %q, must not embed the attacker-supplied candidate", got)
	}
}

func TestNewReturnURLAllowlistNormalizesSuffixWithoutLeadingDot(t *testing.T) {
	a, err := NewReturnURLAllowlist(nil, []string{"mark8ly.com"})
	if err != nil {
		t.Fatalf("NewReturnURLAllowlist: %v", err)
	}
	if _, err := a.ValidateReturnURL("https://shop.mark8ly.com/callback"); err != nil {
		t.Fatalf("suffix without leading dot should still permit subdomains: err = %v", err)
	}
	if _, err := a.ValidateReturnURL("https://evilmark8ly.com/callback"); err == nil {
		t.Fatal("a bare suffix must not become a substring match")
	}
}

func TestEmptyAllowlistRejectsEverything(t *testing.T) {
	a, err := NewReturnURLAllowlist(nil, nil)
	if err != nil {
		t.Fatalf("NewReturnURLAllowlist: %v", err)
	}
	if _, err := a.ValidateReturnURL("https://admin.mark8ly.com/callback"); err == nil {
		t.Fatal("an empty allowlist must fail closed")
	}
}

// Finding 1: a dangerous allowlist entry (bare TLD, stray dot, scheme/path
// smuggled into what must be a bare host) must be rejected at construction,
// never accepted and then silently turned into a no-op at request time.
func TestNewReturnURLAllowlistRejectsDangerousEntries(t *testing.T) {
	tests := []struct {
		name     string
		hosts    []string
		suffixes []string
	}{
		{"bare TLD as a suffix", nil, []string{"com"}},
		{"bare TLD as a suffix with leading dot", nil, []string{".com"}},
		{"stray bare dot as a suffix", nil, []string{"."}},
		{"bare TLD as a host", []string{"com"}, nil},
		{"suffix entry with a scheme", nil, []string{"https://mark8ly.com"}},
		{"suffix entry with a path", nil, []string{"mark8ly.com/callback"}},
		{"suffix entry with userinfo", nil, []string{"user@mark8ly.com"}},
		{"suffix entry with a wildcard", nil, []string{"*.mark8ly.com"}},
		{"suffix entry with whitespace", nil, []string{"mark8ly .com"}},
		{"suffix entry with a doubled dot", nil, []string{"mark8ly..com"}},
		{"host with a doubled dot", []string{"admin..mark8ly.com"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewReturnURLAllowlist(tc.hosts, tc.suffixes)
			if err == nil {
				t.Fatal("NewReturnURLAllowlist = nil error, want rejection of the dangerous entry")
			}
			if !errors.Is(err, ErrReturnURLAllowlistInvalid) {
				t.Fatalf("err = %v, want ErrReturnURLAllowlistInvalid", err)
			}
		})
	}
}

// The construction error must name the offending entry so an operator
// looking at a boot failure can see which config value is wrong.
func TestNewReturnURLAllowlistErrorNamesTheOffendingEntry(t *testing.T) {
	_, err := NewReturnURLAllowlist(nil, []string{"com"})
	if err == nil || !strings.Contains(err.Error(), "com") {
		t.Fatalf("err = %v, want it to name the offending entry", err)
	}
}

func TestNewReturnURLAllowlistAcceptsGoodEntriesAlongsideCollectingAllBadOnes(t *testing.T) {
	_, err := NewReturnURLAllowlist(
		[]string{"admin.mark8ly.com", "com"},
		[]string{"mark8ly.com", "."},
	)
	if err == nil {
		t.Fatal("want an error naming both bad entries")
	}
	if !strings.Contains(err.Error(), `"com"`) || !strings.Contains(err.Error(), `"."`) {
		t.Fatalf("err = %v, want both bad entries named", err)
	}
}
