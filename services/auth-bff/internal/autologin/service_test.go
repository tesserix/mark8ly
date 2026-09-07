package autologin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/auth-bff/internal/audit"
	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
)

const testKey = "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256

func newTestService(t *testing.T, fgaFake *authz.FakeClient, policy RetryPolicy) *Service {
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

// login is the LoginContext every shared-gauntlet test drives
// CompleteForProvider with. Zitadel is the only auth provider, so this is
// the only way into completeLogin.
func login(uid, tenant string) zitadellogin.LoginContext {
	return zitadellogin.LoginContext{
		UID:       uid,
		Email:     uid + "@e.com",
		TenantID:  tenant,
		UserAgent: "TestAgent/1.0",
		Device:    "Browser",
		IPAddress: "203.0.113.7",
		Country:   "IN",
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Happy path
// ─────────────────────────────────────────────────────────────────────────

func TestCompleteForProvider_HappyPath_MintsSession(t *testing.T) {
	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-1", "tenant-uuid-1")

	svc := newTestService(t, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	if _, err := svc.CompleteForProvider(context.Background(), w, login("user-1", "tenant-uuid-1")); err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
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

// TestCompleteForProvider_RetriesUntilMembershipVisible simulates the race:
// at the time the login first calls FGA, the tuple isn't visible yet (because
// the drainer hasn't shipped it). Two ticks later it appears. The retry loop
// must succeed without surfacing the race to the user.
func TestCompleteForProvider_RetriesUntilMembershipVisible(t *testing.T) {
	fgaFake := authz.NewFake()
	// Membership NOT yet set. We'll set it after the first 3 check attempts
	// to simulate the drainer catching up.
	go func() {
		// Wait long enough for ~3 retry attempts under fastPolicy.
		time.Sleep(15 * time.Millisecond)
		fgaFake.SetMembership("user-2", "tenant-uuid-2")
	}()

	svc := newTestService(t, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	if _, err := svc.CompleteForProvider(context.Background(), w, login("user-2", "tenant-uuid-2")); err != nil {
		t.Fatalf("the login should have retried until success, got %v", err)
	}

	// Should have retried multiple times before succeeding.
	if calls := fgaFake.CheckCallCount(); calls < 2 {
		t.Errorf("expected at least 2 FGA check calls (initial + retry), got %d", calls)
	}
}

// TestCompleteForProvider_GivesUpAfterMaxAttempts asserts the retry budget is
// bounded. If the membership tuple is genuinely never going to appear (drainer
// is broken), we return ErrNotMember after MaxAttempts, not loop forever.
func TestCompleteForProvider_GivesUpAfterMaxAttempts(t *testing.T) {
	fgaFake := authz.NewFake()
	// Membership intentionally NEVER set.

	svc := newTestService(t, fgaFake, RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	})
	w := httptest.NewRecorder()

	_, err := svc.CompleteForProvider(context.Background(), w, login("user-3", "tenant-uuid-3"))
	if !errors.Is(err, ErrNotMember) {
		t.Errorf("expected ErrNotMember after retry budget, got %v", err)
	}
	if calls := fgaFake.CheckCallCount(); calls != 3 {
		t.Errorf("expected exactly MaxAttempts (3) check calls, got %d", calls)
	}
}

// TestCompleteForProvider_TransientFGAErrorIsRetried tests the OTHER kind of
// retry: FGA returns an error (network blip), not just "not found". The
// service treats both the same way — back off and try again.
func TestCompleteForProvider_TransientFGAErrorIsRetried(t *testing.T) {
	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-4", "tenant-uuid-4")
	fgaFake.FailNextChecks(2) // first 2 calls fail; 3rd succeeds

	svc := newTestService(t, fgaFake, fastPolicy)
	w := httptest.NewRecorder()

	if _, err := svc.CompleteForProvider(context.Background(), w, login("user-4", "tenant-uuid-4")); err != nil {
		t.Fatalf("the login should retry through transient errors, got %v", err)
	}
}

// TestCompleteForProvider_FGAUnreachable_AfterRetryBudget asserts that
// persistent FGA errors surface as ErrFGAUnreachable.
func TestCompleteForProvider_FGAUnreachable_AfterRetryBudget(t *testing.T) {
	fgaFake := authz.NewFake()
	fgaFake.FailNextChecks(100) // way more than the retry budget

	svc := newTestService(t, fgaFake, RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	})
	w := httptest.NewRecorder()

	_, err := svc.CompleteForProvider(context.Background(), w, login("user-5", "tenant-uuid-5"))
	if !errors.Is(err, ErrFGAUnreachable) {
		t.Errorf("expected ErrFGAUnreachable, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Unwired authz client
// ─────────────────────────────────────────────────────────────────────────

// Regression: a nil FGA client used to dereference into a recovered panic,
// which Gin turned into a 500 on every sign-in. It must report the outage
// as ErrFGAUnreachable so the caller answers with an outage, and no session
// is minted.
func TestCompleteForProvider_NilFGAClient_ReportsUnreachableNotPanic(t *testing.T) {
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
		FGA:      nil, // openfga never resolved at startup
		Sessions: sm,
		Policy:   fastPolicy,
	})

	w := httptest.NewRecorder()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CompleteForProvider panicked with a nil FGA client: %v", r)
		}
	}()

	_, err = svc.CompleteForProvider(context.Background(), w, login("user-1", "tenant-uuid-1"))
	if !errors.Is(err, ErrFGAUnreachable) {
		t.Fatalf("err = %v, want ErrFGAUnreachable", err)
	}

	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "m8_") && c.Value != "" {
			t.Error("session cookie minted despite unreachable authz")
		}
	}
}

func TestCompleteForProvider_RunsTheSameGauntletAsCompleteLogin(t *testing.T) {
	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-z1", "tenant-uuid-1")

	svc := newTestService(t, fgaFake, fastPolicy)

	rec := httptest.NewRecorder()
	_, err := svc.CompleteForProvider(context.Background(), rec, zitadellogin.LoginContext{
		UID:       "user-z1",
		Email:     "z@e.com",
		TenantID:  "tenant-uuid-1",
		UserAgent: "ZitadelTestAgent/1.0",
		IPAddress: "203.0.113.7",
		Device:    "Browser",
		Country:   "IN",
	})
	if err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("CompleteForProvider minted no cookie")
	}
}

func TestCompleteForProvider_PropagatesMembershipFailure(t *testing.T) {
	fgaFake := authz.NewFake() // no membership set

	svc := newTestService(t, fgaFake, fastPolicy)

	rec := httptest.NewRecorder()
	_, err := svc.CompleteForProvider(context.Background(), rec, zitadellogin.LoginContext{
		UID:      "user-z2",
		Email:    "z2@e.com",
		TenantID: "tenant-uuid-1",
	})
	if !errors.Is(err, ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

// TestCompleteForProviderLabelsTheAuditEventAsZitadel pins the fix for the
// audit "method" field: every login used to record "auto_login", even a
// Zitadel one, which misattributed the login method in the audit trail.
// CompleteForProvider must now carry "zitadel" through.
func TestCompleteForProviderLabelsTheAuditEventAsZitadel(t *testing.T) {
	events := make(chan audit.Event, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev audit.Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			t.Errorf("decode audit event: %v", err)
		}
		events <- ev
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-z3", "tenant-uuid-1")

	sm, err := session.NewManager(session.Config{
		CookieName: "m8_test", Domain: "localhost", Secure: false, EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	svc := NewService(Config{
		FGA: fgaFake, Sessions: sm, Policy: fastPolicy,
		Audit: audit.New(srv.URL, "", nil),
	})

	rec := httptest.NewRecorder()
	if _, err := svc.CompleteForProvider(context.Background(), rec, zitadellogin.LoginContext{
		UID: "user-z3", Email: "z3@e.com", TenantID: "tenant-uuid-1",
	}); err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}

	select {
	case ev := <-events:
		if got, _ := ev.Metadata["method"].(string); got != "zitadel" {
			t.Fatalf("audit event metadata.method = %q, want %q", got, "zitadel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the audit event")
	}
}
