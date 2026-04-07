package notification

import (
	"bytes"
	"embed"
	"fmt"
	htmltpl "html/template"
	texttpl "text/template"
)

//go:embed templates/*.html templates/*.txt
var templatesFS embed.FS

// EmailVerificationVars are the variables for the magic-link verification email.
type EmailVerificationVars struct {
	BusinessName string // optional — empty for first-touch onboarding
	VerifyURL    string // clickable magic link, e.g. https://onboarding.mark8ly.com/onboarding/verify?token=...
	ExpiresIn    string // human-readable, e.g. "24 hours"
	SupportEmail string
}

// WelcomeVars are the variables for the post-onboarding welcome email.
type WelcomeVars struct {
	BusinessName string
	OwnerName    string
	AdminURL     string
	StorefrontURL string
	SupportEmail string
}

// RenderEmailVerification renders the verification OTP email and returns
// a fully populated Email ready to hand to a Sender.
func RenderEmailVerification(to, from string, vars EmailVerificationVars) (Email, error) {
	html, err := renderHTML("templates/email_verification.html", vars)
	if err != nil {
		return Email{}, err
	}
	text, err := renderText("templates/email_verification.txt", vars)
	if err != nil {
		return Email{}, err
	}
	return Email{
		To:       to,
		From:     from,
		Subject:  "Verify your email — Mark8ly",
		HTMLBody: html,
		TextBody: text,
	}, nil
}

// RenderWelcome renders the welcome email sent on onboarding completion.
func RenderWelcome(to, from string, vars WelcomeVars) (Email, error) {
	html, err := renderHTML("templates/welcome.html", vars)
	if err != nil {
		return Email{}, err
	}
	text, err := renderText("templates/welcome.txt", vars)
	if err != nil {
		return Email{}, err
	}
	return Email{
		To:       to,
		From:     from,
		Subject:  fmt.Sprintf("Welcome to Mark8ly, %s", vars.BusinessName),
		HTMLBody: html,
		TextBody: text,
	}, nil
}

// renderHTML loads, parses, and executes an HTML template with auto-escaping.
func renderHTML(path string, vars any) (string, error) {
	raw, err := templatesFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("notification: read %s: %w", path, err)
	}
	t, err := htmltpl.New(path).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("notification: parse %s: %w", path, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("notification: execute %s: %w", path, err)
	}
	return buf.String(), nil
}

// renderText loads, parses, and executes a plain text template (no escaping).
func renderText(path string, vars any) (string, error) {
	raw, err := templatesFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("notification: read %s: %w", path, err)
	}
	t, err := texttpl.New(path).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("notification: parse %s: %w", path, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("notification: execute %s: %w", path, err)
	}
	return buf.String(), nil
}
