package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestChallengeHost(t *testing.T) {
	if got := ChallengeHost("shop.example.com"); got != "_mark8ly-challenge.shop.example.com" {
		t.Fatalf("ChallengeHost = %q", got)
	}
	if got := ChallengeHost("Shop.Example.COM."); got != "_mark8ly-challenge.shop.example.com" {
		t.Fatalf("ChallengeHost does not canonicalise: %q", got)
	}
}

func TestChallengeToken_StableAndScoped(t *testing.T) {
	tenantA, tenantB := uuid.New(), uuid.New()
	const secret = "challenge-secret"

	tok := ChallengeToken(secret, tenantA, "shop.example.com")

	if tok != ChallengeToken(secret, tenantA, "shop.example.com") {
		t.Fatal("token is not stable across calls")
	}
	if tok != ChallengeToken(secret, tenantA, "SHOP.example.com.") {
		t.Fatal("token is not stable across host spellings")
	}
	if tok == ChallengeToken(secret, tenantB, "shop.example.com") {
		t.Fatal("two tenants share a token for the same domain")
	}
	if tok == ChallengeToken(secret, tenantA, "other.example.com") {
		t.Fatal("two domains share a token for the same tenant")
	}
	if tok == ChallengeToken("different-secret", tenantA, "shop.example.com") {
		t.Fatal("token does not depend on the server secret")
	}
	if !strings.HasPrefix(tok, ChallengePrefix) {
		t.Fatalf("token %q lacks the %q prefix", tok, ChallengePrefix)
	}
	// Long enough that guessing is not a strategy.
	if len(tok) < len(ChallengePrefix)+32 {
		t.Fatalf("token %q is too short", tok)
	}
}

func TestChallengeMatches(t *testing.T) {
	tenant := uuid.New()
	const secret = "challenge-secret"
	tok := ChallengeToken(secret, tenant, "shop.example.com")

	cases := []struct {
		name    string
		records []string
		want    bool
	}{
		{"exact record", []string{tok}, true},
		{"among unrelated records", []string{"v=spf1 -all", tok, "google-site-verification=x"}, true},
		{"whitespace tolerated", []string{"  " + tok + "  "}, true},
		{"quoted by the resolver", []string{`"` + tok + `"`}, true},
		{"no records", nil, false},
		{"unrelated records only", []string{"v=spf1 -all"}, false},
		{"prefix present but wrong digest", []string{ChallengePrefix + "deadbeef"}, false},
		{"another tenant's token", []string{ChallengeToken(secret, uuid.New(), "shop.example.com")}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChallengeMatches(tc.records, tok); got != tc.want {
				t.Fatalf("ChallengeMatches(%v) = %v, want %v", tc.records, got, tc.want)
			}
		})
	}
}
