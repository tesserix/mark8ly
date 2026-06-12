package mailer

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
)

// recordingMailer captures every SendOTP call and returns a scripted error.
type recordingMailer struct {
	calls int
	to    string
	err   error
}

func (r *recordingMailer) SendOTP(_ context.Context, _, to, _, _, _ string) error {
	r.calls++
	r.to = to
	return r.err
}

func TestFallbackMailer_PrimarySucceeds_FallbackNotCalled(t *testing.T) {
	primary := &recordingMailer{}
	fallback := &recordingMailer{}
	m := NewFallbackChain(slog.Default(), primary, fallback)

	if err := m.SendOTP(context.Background(), "t1", "user@example.com", "User", "123456", "Store"); err != nil {
		t.Fatalf("SendOTP: %v", err)
	}
	if primary.calls != 1 || fallback.calls != 0 {
		t.Fatalf("calls primary=%d fallback=%d, want 1/0 — fallback must not fire when primary delivers", primary.calls, fallback.calls)
	}
}

func TestFallbackMailer_PrimaryFails_FallbackDelivers(t *testing.T) {
	primary := &recordingMailer{err: errors.New("sendgrid 503")}
	fallback := &recordingMailer{}
	m := NewFallbackChain(slog.Default(), primary, fallback)

	if err := m.SendOTP(context.Background(), "t1", "user@example.com", "User", "123456", "Store"); err != nil {
		t.Fatalf("SendOTP: %v — fallback delivery must count as success", err)
	}
	if fallback.calls != 1 || fallback.to != "user@example.com" {
		t.Fatalf("fallback calls=%d to=%q, want the original message retried once", fallback.calls, fallback.to)
	}
}

func TestFallbackMailer_AllFail_ErrorsJoined(t *testing.T) {
	aErr := errors.New("a down")
	bErr := errors.New("b down")
	cErr := errors.New("c down")
	m := NewFallbackChain(slog.Default(),
		&recordingMailer{err: aErr},
		&recordingMailer{err: bErr},
		&recordingMailer{err: cErr},
	)

	err := m.SendOTP(context.Background(), "t1", "user@example.com", "User", "123456", "Store")
	for _, want := range []error{aErr, bErr, cErr} {
		if !errors.Is(err, want) {
			t.Fatalf("error %v must wrap %v", err, want)
		}
	}
}

func TestFallbackMailer_ContextCancelled_SkipsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	primary := &recordingMailer{err: context.Canceled}
	fallback := &recordingMailer{}
	m := NewFallbackChain(slog.Default(), primary, fallback)

	if err := m.SendOTP(ctx, "t1", "user@example.com", "User", "123456", "Store"); err == nil {
		t.Fatal("SendOTP: want error on cancelled context")
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls=%d, want 0 — a dead context must not burn the fallback provider", fallback.calls)
	}
}

func TestResolveOrder(t *testing.T) {
	log := slog.Default()
	cases := []struct {
		order string
		want  []string
	}{
		{"", []string{ProviderSendGrid, ProviderResend}},
		{"sendgrid", []string{ProviderSendGrid, ProviderResend}},
		{"resend", []string{ProviderResend, ProviderSendGrid}},
		{" Resend ", []string{ProviderResend, ProviderSendGrid}},
		{"resend,sendgrid", []string{ProviderResend, ProviderSendGrid}},
		{"bogus", []string{ProviderSendGrid, ProviderResend}}, // typo must not take OTP mail down
		{"bogus,resend", []string{ProviderResend, ProviderSendGrid}},
	}
	for _, c := range cases {
		if got := resolveOrder(c.order, log); !reflect.DeepEqual(got, c.want) {
			t.Errorf("resolveOrder(%q) = %v, want %v", c.order, got, c.want)
		}
	}
}

func TestNewFromConfig_Modes(t *testing.T) {
	log := slog.Default()
	both := map[string]string{ProviderSendGrid: "sg", ProviderResend: "re"}
	if _, ok := NewFromConfig(both, "resend", "noreply@x.com", "X", log).(*FallbackMailer); !ok {
		t.Fatal("two keys: want *FallbackMailer chain")
	}
	if _, ok := NewFromConfig(map[string]string{ProviderSendGrid: "sg"}, "resend", "noreply@x.com", "X", log).(*SendgridMailer); !ok {
		t.Fatal("sendgrid key only: want bare *SendgridMailer even when order says resend")
	}
	if _, ok := NewFromConfig(map[string]string{ProviderResend: "re"}, "sendgrid", "noreply@x.com", "X", log).(*ResendMailer); !ok {
		t.Fatal("resend key only: want bare *ResendMailer")
	}
	if _, ok := NewFromConfig(nil, "resend", "noreply@x.com", "X", log).(*LogMailer); !ok {
		t.Fatal("no keys: want *LogMailer")
	}
}
