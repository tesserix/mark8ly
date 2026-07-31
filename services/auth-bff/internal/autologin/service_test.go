package autologin

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/gip"
	"github.com/mark8ly/auth-bff/internal/session"
)

const testKey = "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256

func newTestService(t *testing.T, gipFake *gip.FakeVerifier, fgaFake *authz.FakeClient, policy RetryPolicy) *Service {
	t.Helper()
	sm, err := session.NewManager(session.Config{
		CookieName: "m8_test",
		Domain:     "localhost",
		Secure:     false,
		EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	return NewService(Config{
		GIP:      gipFake,
		FGA:      fgaFake,
		Sessions: sm,
		Policy:   policy,
	})
}

// fastPolicy lets retry tests run in milliseconds, not seconds.
var fastPolicy = RetryPolicy{
	MaxAttempts:    8,
	InitialBackoff: time.Millisecond,
	MaxBackoff:     5 * time.Millisecond,
}

// ─────────────────────────────────────────────────────────────────────────
// Happy path
// ─────────────────────────────────────────────────────────────────────────

func TestAutoLogin_HappyPath_MintsSession(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{
		UID:      "user-1",
		Email:    "u@e.com",
		TenantID: "MP-Internal-test",
	})

	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-1", "tenant-uuid-1")

	svc := newTestService(t, gipFake, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	res, err := svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "good-token",
		ExpectedTenantID: "MP-Internal-test",
		WorkspaceTenant:  "tenant-uuid-1",
	})
	if err != nil {
		t.Fatalf("AutoLogin: %v", err)
	}
	if res.UID != "user-1" {
		t.Errorf("UID = %q, want user-1", res.UID)
	}

	// A session cookie must have been minted.
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie header was written")
	}
	found := false
	for _, c := range cookies {
		if c.Name == "m8_test" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected m8_test cookie with non-empty value")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// auth-bug #2 regression: race window closure via retry
// ─────────────────────────────────────────────────────────────────────────

// TestAutoLogin_RetriesUntilMembershipVisible simulates the race: at the
// time autologin first calls FGA, the tuple isn't visible yet (because the
// drainer hasn't shipped it). Two ticks later it appears. The retry loop
// must succeed without surfacing the race to the user.
func TestAutoLogin_RetriesUntilMembershipVisible(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{
		UID:      "user-2",
		TenantID: "MP-Internal-test",
	})

	fgaFake := authz.NewFake()
	// Membership NOT yet set. We'll set it after the first 3 check attempts
	// to simulate the drainer catching up.
	go func() {
		// Wait long enough for ~3 retry attempts under fastPolicy.
		time.Sleep(15 * time.Millisecond)
		fgaFake.SetMembership("user-2", "tenant-uuid-2")
	}()

	svc := newTestService(t, gipFake, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	res, err := svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "good-token",
		ExpectedTenantID: "MP-Internal-test",
		WorkspaceTenant:  "tenant-uuid-2",
	})
	if err != nil {
		t.Fatalf("AutoLogin should have retried until success, got %v", err)
	}
	if res == nil {
		t.Fatal("res = nil")
	}

	// Should have retried multiple times before succeeding.
	if calls := fgaFake.CheckCallCount(); calls < 2 {
		t.Errorf("expected at least 2 FGA check calls (initial + retry), got %d", calls)
	}
}

// TestAutoLogin_GivesUpAfterMaxAttempts asserts the retry budget is bounded.
// If the membership tuple is genuinely never going to appear (drainer is
// broken), we return ErrNotMember after MaxAttempts, not loop forever.
func TestAutoLogin_GivesUpAfterMaxAttempts(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{
		UID:      "user-3",
		TenantID: "MP-Internal-test",
	})

	fgaFake := authz.NewFake()
	// Membership intentionally NEVER set.

	svc := newTestService(t, gipFake, fgaFake, RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	})
	w := httptest.NewRecorder()

	_, err := svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "good-token",
		ExpectedTenantID: "MP-Internal-test",
		WorkspaceTenant:  "tenant-uuid-3",
	})
	if !errors.Is(err, ErrNotMember) {
		t.Errorf("expected ErrNotMember after retry budget, got %v", err)
	}
	if calls := fgaFake.CheckCallCount(); calls != 3 {
		t.Errorf("expected exactly MaxAttempts (3) check calls, got %d", calls)
	}
}

// TestAutoLogin_TransientFGAErrorIsRetried tests the OTHER kind of retry:
// FGA returns an error (network blip), not just "not found". The autologin
// service treats both the same way — back off and try again.
func TestAutoLogin_TransientFGAErrorIsRetried(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{
		UID:      "user-4",
		TenantID: "MP-Internal-test",
	})

	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-4", "tenant-uuid-4")
	fgaFake.FailNextChecks(2) // first 2 calls fail; 3rd succeeds

	svc := newTestService(t, gipFake, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	res, err := svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "good-token",
		ExpectedTenantID: "MP-Internal-test",
		WorkspaceTenant:  "tenant-uuid-4",
	})
	if err != nil {
		t.Fatalf("AutoLogin should retry through transient errors, got %v", err)
	}
	if res == nil {
		t.Fatal("res = nil")
	}
}

// TestAutoLogin_FGAUnreachable_AfterRetryBudget asserts that persistent FGA
// errors surface as ErrFGAUnreachable (which the handler maps to 503).
func TestAutoLogin_FGAUnreachable_AfterRetryBudget(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{
		UID:      "user-5",
		TenantID: "MP-Internal-test",
	})

	fgaFake := authz.NewFake()
	fgaFake.FailNextChecks(100) // way more than the retry budget

	svc := newTestService(t, gipFake, fgaFake, RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	})
	w := httptest.NewRecorder()

	_, err := svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "good-token",
		ExpectedTenantID: "MP-Internal-test",
		WorkspaceTenant:  "tenant-uuid-5",
	})
	if !errors.Is(err, ErrFGAUnreachable) {
		t.Errorf("expected ErrFGAUnreachable, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// auth-bug #5 regression: tenant mismatch is enforced before FGA check
// ─────────────────────────────────────────────────────────────────────────

// TestAutoLogin_RejectsWrongTenantPool asserts that a token issued for the
// storefront pool can't be used to log into the admin app even if the user
// is technically a member of the workspace tenant.
func TestAutoLogin_RejectsWrongTenantPool(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("storefront-token", gip.VerifiedToken{
		UID:      "user-6",
		TenantID: "MP-Customer-test", // wrong pool
	})

	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-6", "tenant-uuid-6")

	svc := newTestService(t, gipFake, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	_, err := svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "storefront-token",
		ExpectedTenantID: "MP-Internal-test", // expecting admin pool
		WorkspaceTenant:  "tenant-uuid-6",
	})
	if !errors.Is(err, ErrTenantMismatch) {
		t.Errorf("expected ErrTenantMismatch (auth-bug #5 regression), got %v", err)
	}

	// And no session cookie should have been minted.
	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "m8_") && c.Value != "" {
			t.Error("session cookie was minted despite tenant mismatch")
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Input validation
// ─────────────────────────────────────────────────────────────────────────

func TestAutoLogin_RejectsEmptyFields(t *testing.T) {
	svc := newTestService(t, gip.NewFakeVerifier(), authz.NewFake(), fastPolicy)
	w := httptest.NewRecorder()

	cases := []Request{
		{}, // all empty
		{IDToken: "x"},
		{IDToken: "x", ExpectedTenantID: "y"},
	}
	for i, req := range cases {
		_, err := svc.AutoLogin(context.Background(), w, req)
		if err == nil {
			t.Errorf("case %d: expected error for incomplete request", i)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Unwired authz client
// ─────────────────────────────────────────────────────────────────────────

// Regression: a nil FGA client used to dereference into a recovered panic,
// which Gin turned into a 500 on every sign-in. It must report the outage
// as ErrFGAUnreachable so the handler answers 503 and clients retry.
func TestAutoLogin_NilFGAClient_ReportsUnreachableNotPanic(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{
		UID:      "user-1",
		Email:    "u@e.com",
		TenantID: "MP-Internal-test",
	})

	sm, err := session.NewManager(session.Config{
		CookieName: "m8_test",
		Domain:     "localhost",
		Secure:     false,
		EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}

	svc := NewService(Config{
		GIP:      gipFake,
		FGA:      nil, // openfga never resolved at startup
		Sessions: sm,
		Policy:   fastPolicy,
	})

	w := httptest.NewRecorder()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AutoLogin panicked with a nil FGA client: %v", r)
		}
	}()

	_, err = svc.AutoLogin(context.Background(), w, Request{
		IDToken:          "good-token",
		ExpectedTenantID: "MP-Internal-test",
		WorkspaceTenant:  "tenant-uuid-1",
	})
	if !errors.Is(err, ErrFGAUnreachable) {
		t.Fatalf("err = %v, want ErrFGAUnreachable", err)
	}

	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "m8_") && c.Value != "" {
			t.Error("session cookie minted despite unreachable authz")
		}
	}
}
