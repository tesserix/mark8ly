package notification

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// recordingSender captures every Send call and returns a scripted error.
type recordingSender struct {
	calls []Email
	err   error
}

func (r *recordingSender) Send(_ context.Context, msg Email) error {
	r.calls = append(r.calls, msg)
	return r.err
}

func testEmail() Email {
	return Email{
		To:       "user@example.com",
		From:     "noreply@mark8ly.com",
		Subject:  "subject",
		TextBody: "body",
	}
}

func TestFallbackSender_PrimarySucceeds_FallbackNotCalled(t *testing.T) {
	primary := &recordingSender{}
	fallback := &recordingSender{}
	s := NewFallbackSender(primary, fallback, slog.Default())

	if err := s.Send(context.Background(), testEmail()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(primary.calls) != 1 {
		t.Fatalf("primary calls = %d, want 1", len(primary.calls))
	}
	if len(fallback.calls) != 0 {
		t.Fatalf("fallback calls = %d, want 0 — fallback must not fire when primary delivers", len(fallback.calls))
	}
}

func TestFallbackSender_PrimaryFails_FallbackDelivers(t *testing.T) {
	primary := &recordingSender{err: errors.New("sendgrid returned 503")}
	fallback := &recordingSender{}
	s := NewFallbackSender(primary, fallback, slog.Default())

	msg := testEmail()
	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v — fallback delivery must count as success", err)
	}
	if len(fallback.calls) != 1 {
		t.Fatalf("fallback calls = %d, want 1", len(fallback.calls))
	}
	if fallback.calls[0] != msg {
		t.Fatalf("fallback received %+v, want the original message %+v", fallback.calls[0], msg)
	}
}

func TestFallbackSender_BothFail_ErrorsJoined(t *testing.T) {
	primaryErr := errors.New("sendgrid returned 503")
	fallbackErr := errors.New("resend returned 500")
	primary := &recordingSender{err: primaryErr}
	fallback := &recordingSender{err: fallbackErr}
	s := NewFallbackSender(primary, fallback, slog.Default())

	err := s.Send(context.Background(), testEmail())
	if err == nil {
		t.Fatal("Send: want error when both providers fail")
	}
	if !errors.Is(err, primaryErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("Send error %v must wrap both provider errors", err)
	}
}

func TestFallbackSender_ContextCancelled_SkipsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	primary := &recordingSender{err: context.Canceled}
	fallback := &recordingSender{}
	s := NewFallbackSender(primary, fallback, slog.Default())

	cancel()
	if err := s.Send(ctx, testEmail()); err == nil {
		t.Fatal("Send: want error on cancelled context")
	}
	if len(fallback.calls) != 0 {
		t.Fatalf("fallback calls = %d, want 0 — a dead context must not burn the fallback provider", len(fallback.calls))
	}
}

func TestFallbackSender_InvalidEmail_NoProviderCalled(t *testing.T) {
	primary := &recordingSender{}
	fallback := &recordingSender{}
	s := NewFallbackSender(primary, fallback, slog.Default())

	if err := s.Send(context.Background(), Email{}); err == nil {
		t.Fatal("Send: want validation error for empty email")
	}
	if len(primary.calls) != 0 || len(fallback.calls) != 0 {
		t.Fatal("no provider should be called for an invalid email")
	}
}
