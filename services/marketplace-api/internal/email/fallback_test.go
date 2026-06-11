package email

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// recordingSender captures every Send call and returns a scripted error.
type recordingSender struct {
	calls []Message
	err   error
}

func (r *recordingSender) Send(_ context.Context, msg Message) error {
	r.calls = append(r.calls, msg)
	return r.err
}

func testMessage() Message {
	return Message{
		From:     "noreply@mark8ly.com",
		To:       "user@example.com",
		Subject:  "subject",
		TextBody: "body",
	}
}

func TestFallbackSender_PrimarySucceeds_FallbackNotCalled(t *testing.T) {
	primary := &recordingSender{}
	fallback := &recordingSender{}
	s := NewFallbackSender(primary, fallback, slog.Default())

	if err := s.Send(context.Background(), testMessage()); err != nil {
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

	msg := testMessage()
	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v — fallback delivery must count as success", err)
	}
	if len(fallback.calls) != 1 {
		t.Fatalf("fallback calls = %d, want 1", len(fallback.calls))
	}
	if fallback.calls[0].To != msg.To || fallback.calls[0].Subject != msg.Subject {
		t.Fatalf("fallback received %+v, want the original message", fallback.calls[0])
	}
}

func TestFallbackSender_BothFail_ErrorsJoined(t *testing.T) {
	primaryErr := errors.New("sendgrid returned 503")
	fallbackErr := errors.New("resend returned 500")
	s := NewFallbackSender(
		&recordingSender{err: primaryErr},
		&recordingSender{err: fallbackErr},
		slog.Default(),
	)

	err := s.Send(context.Background(), testMessage())
	if !errors.Is(err, primaryErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("Send error %v must wrap both provider errors", err)
	}
}

func TestFallbackSender_ContextCancelled_SkipsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	primary := &recordingSender{err: context.Canceled}
	fallback := &recordingSender{}
	s := NewFallbackSender(primary, fallback, slog.Default())

	if err := s.Send(ctx, testMessage()); err == nil {
		t.Fatal("Send: want error on cancelled context")
	}
	if len(fallback.calls) != 0 {
		t.Fatalf("fallback calls = %d, want 0 — a dead context must not burn the fallback provider", len(fallback.calls))
	}
}
