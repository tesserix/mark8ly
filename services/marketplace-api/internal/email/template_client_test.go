package email_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// captureSender records every Message handed to it.
type captureSender struct {
	mu   sync.Mutex
	msgs []email.Message
	err  error
}

func (s *captureSender) Send(_ context.Context, msg email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *captureSender) last(t *testing.T) email.Message {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.msgs) == 0 {
		t.Fatal("no message sent")
	}
	return s.msgs[len(s.msgs)-1]
}

// loaderWith returns a DB-less loader with one key registered.
func loaderWith(key, subject, html, text string) *emailtemplates.Loader {
	l := emailtemplates.NewLoader(nil)
	l.Register(key, emailtemplates.EmbeddedFallback{
		Subject: subject, HTMLBody: html, TextBody: text,
	})
	return l
}

func TestTemplateClient_Send_BuildsEnvelope(t *testing.T) {
	sender := &captureSender{}
	loader := loaderWith("dunning_day_5",
		"Payment failed for {{.store_name}}",
		"<p>Hi {{.store_name}}, day {{.day}}</p>",
		"Hi {{.store_name}}, day {{.day}}")

	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{
		"store_name": "Acme",
		"day":        5,
		"tenant_id":  "tenant-123",
	})
	if err != nil {
		t.Fatalf("Send returned %v, want nil", err)
	}

	msg := sender.last(t)
	if msg.To != "merchant@example.com" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.From != "noreply@mark8ly.com" {
		t.Errorf("From = %q", msg.From)
	}
	if msg.FromName != "Mark8ly Billing" {
		t.Errorf("FromName = %q", msg.FromName)
	}
	if msg.Subject != "Payment failed for Acme" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "day 5") {
		t.Errorf("HTMLBody = %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.TextBody, "day 5") {
		t.Errorf("TextBody = %q", msg.TextBody)
	}
	if msg.CustomArgs["product"] != "mark8ly" {
		t.Errorf("product arg = %q", msg.CustomArgs["product"])
	}
	if msg.CustomArgs["kind"] != "dunning_day_5" {
		t.Errorf("kind arg = %q", msg.CustomArgs["kind"])
	}
	if msg.CustomArgs["tenant_id"] != "tenant-123" {
		t.Errorf("tenant_id arg = %q", msg.CustomArgs["tenant_id"])
	}
}

// The whole point of #381: never report success for mail we did not send.
func TestTemplateClient_Send_UndeliverableNeverReachesSender(t *testing.T) {
	sender := &captureSender{}
	loader := loaderWith("dunning_day_5", "s", "<p>h</p>", "t")
	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	for _, to := range []string{"", "b0a1-uuid-not-an-email", "billing+7f3a@mark8ly.local"} {
		err := c.Send(context.Background(), email.TemplateDunningDay5, to, map[string]any{})
		if err == nil {
			t.Fatalf("Send(%q) = nil, want ErrUndeliverable", to)
		}
		if !errors.Is(err, email.ErrUndeliverable) {
			t.Errorf("Send(%q) err = %v, want ErrUndeliverable", to, err)
		}
	}
	if len(sender.msgs) != 0 {
		t.Errorf("sender received %d messages, want 0", len(sender.msgs))
	}
}

func TestTemplateClient_Send_UnknownKeyIsRenderFailure(t *testing.T) {
	sender := &captureSender{}
	c := email.NewTemplateClient(emailtemplates.NewLoader(nil), sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{})
	if err == nil {
		t.Fatal("Send with unregistered key = nil, want error")
	}
	if !errors.Is(err, email.ErrRender) {
		t.Errorf("err = %v, want ErrRender", err)
	}
	if email.SkipReason(err) != email.ReasonRenderFailed {
		t.Errorf("SkipReason = %q, want %q", email.SkipReason(err), email.ReasonRenderFailed)
	}
}

func TestTemplateClient_Send_TransportFailurePropagates(t *testing.T) {
	sender := &captureSender{err: errors.New("sendgrid 503")}
	loader := loaderWith("dunning_day_5", "s", "<p>h</p>", "t")
	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{})
	if err == nil {
		t.Fatal("Send = nil, want transport error")
	}
	if !errors.Is(err, email.ErrTransport) {
		t.Errorf("err = %v, want ErrTransport", err)
	}
	if email.SkipReason(err) != email.ReasonTransportFailed {
		t.Errorf("SkipReason = %q, want %q", email.SkipReason(err), email.ReasonTransportFailed)
	}
}

func TestSkipReason_UndeliverableWins(t *testing.T) {
	err := email.ValidateRecipient("x@y.local")
	if got := email.SkipReason(err); got != email.ReasonPlaceholderAddress {
		t.Errorf("SkipReason = %q, want %q", got, email.ReasonPlaceholderAddress)
	}
	if got := email.SkipReason(errors.New("boom")); got != email.ReasonUnknown {
		t.Errorf("SkipReason(unrelated) = %q, want %q", got, email.ReasonUnknown)
	}
}

func TestTemplateClient_Send_PreservesUnderlyingCause_Transport(t *testing.T) {
	sentinel := errors.New("sendgrid 503 upstream")
	sender := &captureSender{err: sentinel}
	loader := loaderWith("dunning_day_5", "s", "<p>h</p>", "t")
	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{})
	if !errors.Is(err, email.ErrTransport) {
		t.Errorf("lost the sentinel: %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("lost the underlying cause: %v", err)
	}
}

func TestTemplateClient_Send_PreservesUnderlyingCause_Render(t *testing.T) {
	// Verify the render sentinel is preserved in the error chain.
	// TestTemplateClient_Send_PreservesUnderlyingCause_Transport verifies
	// that both the sentinel and underlying cause are preserved for transport errors.
	// For render, we verify at minimum that errors.Is works for the sentinel.
	emptyLoader := emailtemplates.NewLoader(nil)
	sender := &captureSender{}
	c := email.NewTemplateClient(emptyLoader, sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{})
	if !errors.Is(err, email.ErrRender) {
		t.Errorf("lost the render sentinel: %v", err)
	}
}
