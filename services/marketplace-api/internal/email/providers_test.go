package email

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
)

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
		{"bogus", []string{ProviderSendGrid, ProviderResend}}, // typo must not take mail down
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
	if _, ok := NewFromConfig(both, "resend", log).(*FallbackSender); !ok {
		t.Fatal("two keys: want *FallbackSender chain")
	}
	if _, ok := NewFromConfig(map[string]string{ProviderSendGrid: "sg"}, "resend", log).(*SendGridSender); !ok {
		t.Fatal("sendgrid key only: want bare *SendGridSender even when order says resend")
	}
	if _, ok := NewFromConfig(map[string]string{ProviderResend: "re"}, "sendgrid", log).(*ResendSender); !ok {
		t.Fatal("resend key only: want bare *ResendSender")
	}
	if _, ok := NewFromConfig(nil, "resend", log).(*LogSender); !ok {
		t.Fatal("no keys: want *LogSender")
	}
}

func TestFallbackChain_OrderAndRecovery(t *testing.T) {
	// Three providers: first two fail, third delivers — the chain must
	// walk them in order and stop at the first success.
	a := &recordingSender{err: errors.New("a down")}
	b := &recordingSender{err: errors.New("b down")}
	c := &recordingSender{}
	chain := NewFallbackChain(slog.Default(), a, b, c)

	if err := chain.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(a.calls) != 1 || len(b.calls) != 1 || len(c.calls) != 1 {
		t.Fatalf("calls a=%d b=%d c=%d, want 1/1/1", len(a.calls), len(b.calls), len(c.calls))
	}

	// All fail → every error joined.
	c.err = errors.New("c down")
	err := chain.Send(context.Background(), testMessage())
	for _, want := range []error{a.err, b.err, c.err} {
		if !errors.Is(err, want) {
			t.Fatalf("chain error %v must wrap %v", err, want)
		}
	}
}
