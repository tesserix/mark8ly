package zitadellogin

import (
	"errors"
	"testing"
)

func testAllowlist() ReturnURLAllowlist {
	return NewReturnURLAllowlist(
		[]string{"admin.mark8ly.com"},
		[]string{".mark8ly.com"},
	)
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
	}

	a := testAllowlist()
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
	a := testAllowlist()
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
	a := NewReturnURLAllowlist(nil, []string{"mark8ly.com"})
	if _, err := a.ValidateReturnURL("https://shop.mark8ly.com/callback"); err != nil {
		t.Fatalf("suffix without leading dot should still permit subdomains: err = %v", err)
	}
	if _, err := a.ValidateReturnURL("https://evilmark8ly.com/callback"); err == nil {
		t.Fatal("a bare suffix must not become a substring match")
	}
}

func TestEmptyAllowlistRejectsEverything(t *testing.T) {
	a := NewReturnURLAllowlist(nil, nil)
	if _, err := a.ValidateReturnURL("https://admin.mark8ly.com/callback"); err == nil {
		t.Fatal("an empty allowlist must fail closed")
	}
}
