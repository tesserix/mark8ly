package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/loginotp"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
)

// stubOTPStore is an emailotp.Store whose only interesting behaviour is
// how many codes it reports having issued in the window.
type stubOTPStore struct {
	countInWindow int
	inserted      int
}

func (s *stubOTPStore) Insert(context.Context, emailotp.Record) error { s.inserted++; return nil }
func (s *stubOTPStore) Latest(context.Context, string) (*emailotp.Record, error) {
	return nil, emailotp.ErrNoChallenge
}
func (s *stubOTPStore) IncrementAttempts(context.Context, string) error  { return nil }
func (s *stubOTPStore) Consume(context.Context, string, time.Time) error { return nil }
func (s *stubOTPStore) CountSince(context.Context, string, time.Time) (int, error) {
	return s.countInWindow, nil
}

type stubMailer struct {
	sent int
	err  error
}

func (m *stubMailer) SendLoginCode(context.Context, string, string, time.Duration) error {
	m.sent++
	return m.err
}

func newGate(t *testing.T, store emailotp.Store, mailer loginotp.Mailer) *loginotp.Gate {
	t.Helper()
	svc, err := emailotp.NewService(emailotp.Config{
		Store:  store,
		Pepper: "pepper-long-enough-for-the-minimum-length-check",
	})
	if err != nil {
		t.Fatalf("new emailotp service: %v", err)
	}
	return loginotp.NewGate(svc, mailer, emailotp.DefaultTTL)
}

// The translation this adapter exists for. Without it the mobile resend
// handler cannot tell a spent code budget from a delivery failure, and a
// merchant is left tapping Resend against a wall with generic copy.
func TestChallengeIssuerAdapter_TranslatesRateLimited(t *testing.T) {
	store := &stubOTPStore{countInWindow: emailotp.DefaultMaxPerWindow}
	mailer := &stubMailer{}
	a := challengeIssuerAdapter{newGate(t, store, mailer)}

	err := a.IssueChallenge(context.Background(), "a@b.test", "203.0.113.9")

	if !errors.Is(err, zitadellogin.ErrChallengeRateLimited) {
		t.Fatalf("err = %v, want zitadellogin.ErrChallengeRateLimited", err)
	}
	// The original cause is kept for operators reading logs.
	if !errors.Is(err, emailotp.ErrRateLimited) {
		t.Fatalf("err = %v, want the emailotp cause preserved", err)
	}
	if mailer.sent != 0 {
		t.Fatal("a rate-limited request must not send mail")
	}
}

// Anything else passes through unchanged: only the one failure the handler
// answers differently is translated.
func TestChallengeIssuerAdapter_LeavesOtherFailuresAlone(t *testing.T) {
	mailer := &stubMailer{err: errors.New("smtp down")}
	a := challengeIssuerAdapter{newGate(t, &stubOTPStore{}, mailer)}

	err := a.IssueChallenge(context.Background(), "a@b.test", "203.0.113.9")

	if err == nil {
		t.Fatal("want an error")
	}
	if errors.Is(err, zitadellogin.ErrChallengeRateLimited) {
		t.Fatalf("a delivery failure must not read as a spent budget: %v", err)
	}
}

// The happy path really does mail a code, so a green rate-limit test is
// not passing for want of the gate working at all.
func TestChallengeIssuerAdapter_IssuesWithinBudget(t *testing.T) {
	store := &stubOTPStore{countInWindow: emailotp.DefaultMaxPerWindow - 1}
	mailer := &stubMailer{}
	a := challengeIssuerAdapter{newGate(t, store, mailer)}

	if err := a.IssueChallenge(context.Background(), "a@b.test", "203.0.113.9"); err != nil {
		t.Fatalf("IssueChallenge: %v", err)
	}
	if mailer.sent != 1 || store.inserted != 1 {
		t.Fatalf("sent = %d, inserted = %d; want one of each", mailer.sent, store.inserted)
	}
}
