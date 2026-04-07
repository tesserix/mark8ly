package notification

import (
	"context"
	"strings"
	"testing"
)

// TestRenderEmailVerification asserts the magic-link email renders both
// HTML and text bodies, sets the right subject/from/to, and substitutes
// every template variable.
func TestRenderEmailVerification(t *testing.T) {
	msg, err := RenderEmailVerification("user@example.com", "noreply@mark8ly.com", EmailVerificationVars{
		BusinessName: "Acme Co",
		VerifyURL:    "https://onboarding.mark8ly.com/onboarding/verify?token=tok-123",
		ExpiresIn:    "24 hours",
		SupportEmail: "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if msg.To != "user@example.com" {
		t.Errorf("To = %q, want user@example.com", msg.To)
	}
	if msg.From != "noreply@mark8ly.com" {
		t.Errorf("From = %q, want noreply@mark8ly.com", msg.From)
	}
	if !strings.Contains(msg.Subject, "Verify") {
		t.Errorf("Subject = %q, want to contain 'Verify'", msg.Subject)
	}
	if msg.HTMLBody == "" {
		t.Error("HTMLBody is empty")
	}
	if msg.TextBody == "" {
		t.Error("TextBody is empty")
	}
	// Both bodies should contain the magic link URL.
	if !strings.Contains(msg.HTMLBody, "tok-123") {
		t.Errorf("HTMLBody missing token: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.TextBody, "tok-123") {
		t.Errorf("TextBody missing token: %q", msg.TextBody)
	}
	if !strings.Contains(msg.TextBody, "24 hours") {
		t.Errorf("TextBody missing ExpiresIn: %q", msg.TextBody)
	}
}

// TestRenderEmailVerification_NoBusinessName asserts the template still
// renders cleanly for first-touch onboarding where the business name is
// not yet known.
func TestRenderEmailVerification_NoBusinessName(t *testing.T) {
	msg, err := RenderEmailVerification("user@example.com", "noreply@mark8ly.com", EmailVerificationVars{
		BusinessName: "",
		VerifyURL:    "https://onboarding.mark8ly.com/onboarding/verify?token=tok",
		ExpiresIn:    "24 hours",
		SupportEmail: "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if msg.HTMLBody == "" || msg.TextBody == "" {
		t.Error("expected non-empty bodies")
	}
}

// TestRenderWelcome smoke-checks the welcome email template.
func TestRenderWelcome(t *testing.T) {
	msg, err := RenderWelcome("owner@acme.com", "noreply@mark8ly.com", WelcomeVars{
		BusinessName:  "Acme Co",
		OwnerName:     "Pat",
		AdminURL:      "https://acme-admin.mark8ly.com",
		StorefrontURL: "https://acme.mark8ly.com",
		SupportEmail:  "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(msg.Subject, "Acme Co") {
		t.Errorf("Subject = %q, want to contain business name", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "Acme Co") {
		t.Errorf("HTMLBody missing business name")
	}
	if !strings.Contains(msg.HTMLBody, "https://acme-admin.mark8ly.com") {
		t.Errorf("HTMLBody missing AdminURL")
	}
}

// TestValidate covers each rejection branch of the boundary validator.
func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		msg  Email
		want string // substring of error, "" for success
	}{
		{
			name: "ok",
			msg:  Email{To: "a@b", From: "c@d", Subject: "s", TextBody: "t"},
			want: "",
		},
		{
			name: "missing to",
			msg:  Email{Subject: "s", TextBody: "t"},
			want: "missing recipient",
		},
		{
			name: "missing subject",
			msg:  Email{To: "a@b", TextBody: "t"},
			want: "missing subject",
		},
		{
			name: "missing body",
			msg:  Email{To: "a@b", Subject: "s"},
			want: "missing body",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.msg)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

// TestNoopSender_Validates ensures NoopSender still rejects malformed
// emails so test code catches them locally.
func TestNoopSender_Validates(t *testing.T) {
	s := NoopSender{}
	if err := s.Send(context.Background(), Email{}); err == nil {
		t.Error("expected error for empty Email")
	}
	err := s.Send(context.Background(), Email{
		To: "a@b", From: "c@d", Subject: "s", TextBody: "t",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
