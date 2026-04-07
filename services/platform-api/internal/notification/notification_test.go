package notification

import (
	"context"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────
// Template render tests
// ─────────────────────────────────────────────────────────────────────────

func TestRender_EmailVerification_IncludesOTPCode(t *testing.T) {
	got, err := RenderEmailVerification("user@example.com", "noreply@mark8ly.com", EmailVerificationVars{
		OTPCode:      "123456",
		ExpiresIn:    "10 minutes",
		SupportEmail: "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("RenderEmailVerification error = %v", err)
	}

	if !strings.Contains(got.HTMLBody, "123456") {
		t.Errorf("HTML body missing OTP code")
	}
	if !strings.Contains(got.TextBody, "123456") {
		t.Errorf("text body missing OTP code")
	}
	if !strings.Contains(got.HTMLBody, "10 minutes") {
		t.Errorf("HTML body missing expiry duration")
	}
	if !strings.Contains(got.HTMLBody, "help@mark8ly.com") {
		t.Errorf("HTML body missing support email")
	}
	if got.Subject == "" {
		t.Errorf("subject is empty")
	}
}

func TestRender_EmailVerification_BusinessNameIsConditional(t *testing.T) {
	withName, _ := RenderEmailVerification("u@e.com", "f@e.com", EmailVerificationVars{
		BusinessName: "Acme Co",
		OTPCode:      "111111",
		ExpiresIn:    "10 minutes",
		SupportEmail: "help@e.com",
	})
	if !strings.Contains(withName.HTMLBody, "Acme Co") {
		t.Error("HTML body should include business name when set")
	}

	withoutName, _ := RenderEmailVerification("u@e.com", "f@e.com", EmailVerificationVars{
		OTPCode:      "111111",
		ExpiresIn:    "10 minutes",
		SupportEmail: "help@e.com",
	})
	if strings.Contains(withoutName.HTMLBody, "Welcome to Mark8ly,") {
		t.Error("HTML body should NOT include the welcome prefix when business name is empty")
	}
}

func TestRender_EmailVerification_HTMLEscapesUnsafeInput(t *testing.T) {
	// html/template should auto-escape <script> tags so injecting them via
	// a variable can't cause XSS in webmail clients.
	got, err := RenderEmailVerification("u@e.com", "f@e.com", EmailVerificationVars{
		BusinessName: "<script>alert(1)</script>",
		OTPCode:      "111111",
		ExpiresIn:    "10 minutes",
		SupportEmail: "help@e.com",
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(got.HTMLBody, "<script>alert(1)</script>") {
		t.Error("HTML body should escape <script> tags")
	}
	if !strings.Contains(got.HTMLBody, "&lt;script&gt;") {
		t.Error("HTML body should contain the escaped form")
	}
}

func TestRender_Welcome_IncludesAllLinks(t *testing.T) {
	got, err := RenderWelcome("user@example.com", "noreply@mark8ly.com", WelcomeVars{
		BusinessName:  "Acme",
		OwnerName:     "Jane",
		AdminURL:      "https://acme-admin.mark8ly.com",
		StorefrontURL: "https://acme.mark8ly.com",
		SupportEmail:  "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	for _, want := range []string{"Acme", "Jane", "acme-admin.mark8ly.com", "acme.mark8ly.com"} {
		if !strings.Contains(got.HTMLBody, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(got.TextBody, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if !strings.Contains(got.Subject, "Acme") {
		t.Errorf("subject should reference business name, got %q", got.Subject)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Sender tests
// ─────────────────────────────────────────────────────────────────────────

func TestNoopSender_RejectsInvalidEmail(t *testing.T) {
	if err := (NoopSender{}).Send(context.Background(), Email{}); err == nil {
		t.Error("NoopSender should reject empty Email")
	}
	if err := (NoopSender{}).Send(context.Background(), Email{To: "u@e.com"}); err == nil {
		t.Error("NoopSender should reject Email without subject")
	}
	if err := (NoopSender{}).Send(context.Background(), Email{To: "u@e.com", Subject: "x"}); err == nil {
		t.Error("NoopSender should reject Email without body")
	}
}

func TestNoopSender_AcceptsValidEmail(t *testing.T) {
	err := (NoopSender{}).Send(context.Background(), Email{
		To:       "u@e.com",
		From:     "f@e.com",
		Subject:  "x",
		TextBody: "hi",
	})
	if err != nil {
		t.Errorf("NoopSender.Send error = %v", err)
	}
}

func TestSendGridSender_FailsWhenAPIKeyEmpty(t *testing.T) {
	s := NewSendGridSender("")
	err := s.Send(context.Background(), Email{
		To:       "u@e.com",
		From:     "f@e.com",
		Subject:  "x",
		TextBody: "hi",
	})
	if err == nil {
		t.Error("SendGridSender should fail-fast when API key is empty")
	}
}
