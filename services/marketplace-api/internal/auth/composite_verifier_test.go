package auth

import (
	"context"
	"errors"
	"testing"
)

type compositeStubVerifier struct {
	claims *TokenClaims
	err    error
	calls  int
}

func (s *compositeStubVerifier) Verify(_ context.Context, _ string) (*TokenClaims, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.claims, nil
}

func TestCompositeVerifier_FirstIssuerWins(t *testing.T) {
	zit := &compositeStubVerifier{claims: &TokenClaims{UserID: "zit-user"}}
	gip := &compositeStubVerifier{claims: &TokenClaims{UserID: "gip-user", TenantID: "t1"}}

	var seen []string
	v := NewCompositeVerifier([]NamedVerifier{
		{Issuer: "zitadel", Verifier: zit},
		{Issuer: "gip", Verifier: gip},
	}, func(issuer string) { seen = append(seen, issuer) })

	claims, err := v.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.UserID != "zit-user" {
		t.Fatalf("want zit-user, got %q", claims.UserID)
	}
	// The second verifier must not be consulted once the first succeeds:
	// each extra call is a network round-trip on the request path.
	if gip.calls != 0 {
		t.Fatalf("fallback verifier was called %d times, want 0", gip.calls)
	}
	if len(seen) != 1 || seen[0] != "zitadel" {
		t.Fatalf("want one 'zitadel' observation, got %v", seen)
	}
}

func TestCompositeVerifier_FallsBackToSecondIssuer(t *testing.T) {
	zit := &compositeStubVerifier{err: errors.New("token not issued by zitadel")}
	gip := &compositeStubVerifier{claims: &TokenClaims{UserID: "gip-user", TenantID: "t1"}}

	var seen []string
	v := NewCompositeVerifier([]NamedVerifier{
		{Issuer: "zitadel", Verifier: zit},
		{Issuer: "gip", Verifier: gip},
	}, func(issuer string) { seen = append(seen, issuer) })

	claims, err := v.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.UserID != "gip-user" || claims.TenantID != "t1" {
		t.Fatalf("want the GIP claims through unchanged, got %+v", claims)
	}
	// This is the observation that makes the GIP drain measurable, and
	// therefore makes "is it safe to delete GIP?" (#708) answerable.
	if len(seen) != 1 || seen[0] != "gip" {
		t.Fatalf("want one 'gip' observation, got %v", seen)
	}
}

func TestCompositeVerifier_AllFail_ReturnsFirstError(t *testing.T) {
	first := errors.New("zitadel says no")
	zit := &compositeStubVerifier{err: first}
	gip := &compositeStubVerifier{err: errors.New("gip says no")}

	var seen []string
	v := NewCompositeVerifier([]NamedVerifier{
		{Issuer: "zitadel", Verifier: zit},
		{Issuer: "gip", Verifier: gip},
	}, func(issuer string) { seen = append(seen, issuer) })

	_, err := v.Verify(context.Background(), "tok")
	if !errors.Is(err, first) {
		t.Fatalf("want the first issuer's error preserved, got %v", err)
	}
	if gip.calls != 1 {
		t.Fatalf("every issuer must be tried before failing; gip calls=%d", gip.calls)
	}
	if len(seen) != 0 {
		t.Fatalf("a failed verification must record no issuer, got %v", seen)
	}
}

func TestCompositeVerifier_NilObserverIsSafe(t *testing.T) {
	// The observer is optional wiring; forgetting it must not panic on the
	// request path.
	v := NewCompositeVerifier([]NamedVerifier{
		{Issuer: "zitadel", Verifier: &compositeStubVerifier{claims: &TokenClaims{UserID: "u"}}},
	}, nil)
	if _, err := v.Verify(context.Background(), "tok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompositeVerifier_SkipsNilVerifiers(t *testing.T) {
	// Wiring builds this list conditionally (Zitadel may be unconfigured),
	// so a nil entry must be skipped rather than panic.
	gip := &compositeStubVerifier{claims: &TokenClaims{UserID: "gip-user"}}
	v := NewCompositeVerifier([]NamedVerifier{
		{Issuer: "zitadel", Verifier: nil},
		{Issuer: "gip", Verifier: gip},
	}, nil)

	claims, err := v.Verify(context.Background(), "tok")
	if err != nil || claims.UserID != "gip-user" {
		t.Fatalf("want the GIP verifier used, got claims=%+v err=%v", claims, err)
	}
}
