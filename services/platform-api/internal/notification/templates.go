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
	BusinessName  string
	OwnerName     string
	AdminURL      string
	StorefrontURL string
	SupportEmail  string
}

// InvitationVars are the variables for the teammate invitation email.
type InvitationVars struct {
	TenantName   string
	Role         string
	Inviter      string
	AcceptURL    string
	ExpiresIn    string
	SupportEmail string
}

// PasswordResetVars are the variables for the merchant password reset email.
type PasswordResetVars struct {
	ResetURL     string // full URL the user clicks to land on the admin reset page
	ExpiresIn    string // human-readable, e.g. "1 hour"
	SupportEmail string
}

// LoginOTPVars are the variables for the email one-time sign-in code.
type LoginOTPVars struct {
	Code         string // six digits
	ExpiresIn    string // human-readable, e.g. "5 minutes"
	SupportEmail string
}

// NewDeviceLoginVars are the variables for the unrecognised-device alert.
// Device derives from the User-Agent header and is therefore attacker
// influenced — it must only ever be rendered through html/template.
type NewDeviceLoginVars struct {
	Device       string
	CountryName  string // already humanised, or "an unknown location"
	IPAddress    string // optional — omitted from the email when empty
	At           string // formatted timestamp including zone
	SecureURL    string
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

// RenderInvitation renders the "you've been invited" email sent to
// teammates who get added to a tenant.
func RenderInvitation(to, from string, vars InvitationVars) (Email, error) {
	html, err := renderHTML("templates/invitation.html", vars)
	if err != nil {
		return Email{}, err
	}
	text, err := renderText("templates/invitation.txt", vars)
	if err != nil {
		return Email{}, err
	}
	return Email{
		To:       to,
		From:     from,
		Subject:  fmt.Sprintf("You've been invited to join %s on Mark8ly", vars.TenantName),
		HTMLBody: html,
		TextBody: text,
	}, nil
}

// RenderPasswordReset renders the branded password reset email sent
// from platform-api (not GIP's default template).
func RenderPasswordReset(to, from string, vars PasswordResetVars) (Email, error) {
	html, err := renderHTML("templates/password_reset.html", vars)
	if err != nil {
		return Email{}, err
	}
	text, err := renderText("templates/password_reset.txt", vars)
	if err != nil {
		return Email{}, err
	}
	return Email{
		To:       to,
		From:     from,
		Subject:  "Reset your Mark8ly password",
		HTMLBody: html,
		TextBody: text,
	}, nil
}

// RenderLoginOTP renders the email one-time sign-in code. The code is in
// the subject line so it previews on a lock screen without opening the
// message.
func RenderLoginOTP(to, from string, vars LoginOTPVars) (Email, error) {
	html, err := renderHTML("templates/login_otp.html", vars)
	if err != nil {
		return Email{}, err
	}
	text, err := renderText("templates/login_otp.txt", vars)
	if err != nil {
		return Email{}, err
	}
	return Email{
		To:       to,
		From:     from,
		Subject:  fmt.Sprintf("%s is your Mark8ly sign-in code", vars.Code),
		HTMLBody: html,
		TextBody: text,
	}, nil
}

// RenderNewDeviceLogin renders the alert sent when an account is used
// from a device fingerprint with no history.
func RenderNewDeviceLogin(to, from string, vars NewDeviceLoginVars) (Email, error) {
	html, err := renderHTML("templates/new_device_login.html", vars)
	if err != nil {
		return Email{}, err
	}
	text, err := renderText("templates/new_device_login.txt", vars)
	if err != nil {
		return Email{}, err
	}
	return Email{
		To:       to,
		From:     from,
		Subject:  "New sign-in to your Mark8ly account",
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
