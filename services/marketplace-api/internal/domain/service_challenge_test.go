package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var errNoSuchHost = errors.New("no such host")

// stubDNS answers from fixed maps so verification is testable without
// real DNS. A missing key resolves to NXDOMAIN-ish (an error).
type stubDNS struct {
	cname map[string]string
	hosts map[string][]string
	txt   map[string][]string
}

func (s stubDNS) LookupCNAME(_ context.Context, host string) (string, error) {
	if v, ok := s.cname[host]; ok {
		return v, nil
	}
	return "", errNoSuchHost
}

func (s stubDNS) LookupHost(_ context.Context, host string) ([]string, error) {
	if v, ok := s.hosts[host]; ok {
		return v, nil
	}
	return nil, errNoSuchHost
}

func (s stubDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := s.txt[name]; ok {
		return v, nil
	}
	return nil, errNoSuchHost
}

const challengeTestSecret = "test-challenge-secret"

func newChallengeDomain(t *testing.T) *CustomDomain {
	t.Helper()
	target := "edge.mark8ly.com"
	return &CustomDomain{
		ID:          uuid.New(),
		TenantID:    uuid.New(),
		Domain:      "shop.example.com",
		DNSMethod:   DNSMethodManual,
		CnameTarget: &target,
		Status:      DomainStatusPending,
	}
}

func newChallengeService(t *testing.T, repo Repository, dns DNSResolver) *Service {
	t.Helper()
	return NewService(ServiceConfig{
		Repo:            repo,
		DNS:             dns,
		ChallengeSecret: challengeTestSecret,
		Logger:          newSilentLogger(),
	})
}

func TestVerifyManual_RejectsWhenChallengeMissing(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	repo := newMemRepo(d)
	dns := stubDNS{
		cname: map[string]string{d.Domain: "edge.mark8ly.com."},
	}

	got, err := newChallengeService(t, repo, dns).Verify(context.Background(), uuid.New(), d.ID)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got.Status != DomainStatusVerifying {
		t.Fatalf("status = %q, want %q — correct CNAME alone must not verify ownership", got.Status, DomainStatusVerifying)
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, ChallengeHost(d.Domain)) {
		t.Fatalf("error message = %v, want it to name the TXT record to publish", got.ErrorMessage)
	}
}

func TestVerifyManual_RejectsAnotherTenantsToken(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	repo := newMemRepo(d)
	dns := stubDNS{
		cname: map[string]string{d.Domain: "edge.mark8ly.com."},
		txt: map[string][]string{
			ChallengeHost(d.Domain): {ChallengeToken(challengeTestSecret, uuid.New(), d.Domain)},
		},
	}

	got, err := newChallengeService(t, repo, dns).Verify(context.Background(), uuid.New(), d.ID)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got.Status != DomainStatusVerifying {
		t.Fatalf("status = %q, want %q — a token issued to another tenant must not verify", got.Status, DomainStatusVerifying)
	}
}

func TestVerifyManual_AcceptsCNAMEWithChallenge(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	repo := newMemRepo(d)
	dns := stubDNS{
		cname: map[string]string{d.Domain: "edge.mark8ly.com."},
		txt: map[string][]string{
			ChallengeHost(d.Domain): {"v=spf1 -all", ChallengeToken(challengeTestSecret, d.TenantID, d.Domain)},
		},
	}

	got, err := newChallengeService(t, repo, dns).Verify(context.Background(), uuid.New(), d.ID)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got.Status != DomainStatusActive {
		t.Fatalf("status = %q, want %q", got.Status, DomainStatusActive)
	}
}

// The A-record fallback is the weakest routing proof — a shared CDN edge
// IP is not evidence of control — so it must still carry the token.
func TestVerifyManual_ARecordFallbackStillNeedsChallenge(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	repo := newMemRepo(d)
	dns := stubDNS{
		hosts: map[string][]string{
			d.Domain:           {"203.0.113.10"},
			"edge.mark8ly.com": {"203.0.113.10"},
		},
	}

	got, err := newChallengeService(t, repo, dns).Verify(context.Background(), uuid.New(), d.ID)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got.Status != DomainStatusVerifying {
		t.Fatalf("status = %q, want %q", got.Status, DomainStatusVerifying)
	}
}

// A domain that was verified under the old rules keeps serving; forcing
// a re-proof would take a live storefront down on the next refresh.
func TestVerifyManual_AlreadyVerifiedDomainIsGrandfathered(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	now := time.Now()
	d.Status = DomainStatusActive
	d.VerifiedAt = &now
	repo := newMemRepo(d)
	dns := stubDNS{cname: map[string]string{d.Domain: "edge.mark8ly.com."}}

	got, err := newChallengeService(t, repo, dns).Verify(context.Background(), uuid.New(), d.ID)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got.Status != DomainStatusActive {
		t.Fatalf("status = %q, want %q", got.Status, DomainStatusActive)
	}
}

// Local dev and tests run without a challenge secret; the proof is
// skipped there rather than making verification impossible.
func TestVerifyManual_SkipsChallengeWhenUnconfigured(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	repo := newMemRepo(d)
	dns := stubDNS{cname: map[string]string{d.Domain: "edge.mark8ly.com."}}
	svc := NewService(ServiceConfig{Repo: repo, DNS: dns, Logger: newSilentLogger()})

	got, err := svc.Verify(context.Background(), uuid.New(), d.ID)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if got.Status != DomainStatusActive {
		t.Fatalf("status = %q, want %q", got.Status, DomainStatusActive)
	}
}

func TestService_Challenge(t *testing.T) {
	t.Parallel()
	d := newChallengeDomain(t)
	svc := newChallengeService(t, newMemRepo(d), stubDNS{})

	host, token, required := svc.Challenge(d)
	if !required {
		t.Fatal("required = false, want true for an unverified domain with a secret configured")
	}
	if host != ChallengeHost(d.Domain) || token != ChallengeToken(challengeTestSecret, d.TenantID, d.Domain) {
		t.Fatalf("Challenge() = (%q, %q), want the record the merchant must publish", host, token)
	}

	now := time.Now()
	d.VerifiedAt = &now
	if _, _, required := svc.Challenge(d); required {
		t.Fatal("required = true for an already-verified domain, want false")
	}

	unconfigured := NewService(ServiceConfig{Repo: newMemRepo(nil), Logger: newSilentLogger()})
	if _, _, required := unconfigured.Challenge(newChallengeDomain(t)); required {
		t.Fatal("required = true without a challenge secret, want false")
	}
}
