package notification

import (
	"strings"
	"testing"
)

// TestRenderLoginOTP asserts the one-time code reaches both bodies and
// that the expiry is stated, since a code without a stated lifetime is
// the most common support complaint on OTP mail.
func TestRenderLoginOTP(t *testing.T) {
	msg, err := RenderLoginOTP("user@example.com", "noreply@mark8ly.com", LoginOTPVars{
		Code:         "482913",
		ExpiresIn:    "5 minutes",
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
	if !strings.Contains(msg.Subject, "482913") {
		t.Errorf("Subject = %q, want the code inline so it previews on a lock screen", msg.Subject)
	}
	for name, body := range map[string]string{"HTMLBody": msg.HTMLBody, "TextBody": msg.TextBody} {
		if body == "" {
			t.Fatalf("%s is empty", name)
		}
		if !strings.Contains(body, "482913") {
			t.Errorf("%s missing code: %q", name, body)
		}
		if !strings.Contains(body, "5 minutes") {
			t.Errorf("%s missing ExpiresIn: %q", name, body)
		}
	}
}

// TestRenderNewDeviceLogin asserts every fact an account holder needs to
// judge whether the sign-in was theirs survives into both bodies.
func TestRenderNewDeviceLogin(t *testing.T) {
	msg, err := RenderNewDeviceLogin("user@example.com", "noreply@mark8ly.com", NewDeviceLoginVars{
		Device:       "Chrome on macOS",
		CountryName:  "India",
		IPAddress:    "203.0.113.9",
		At:           "9 Aug 2026 at 01:19 IST",
		SecureURL:    "https://admin.mark8ly.com/settings/security",
		SupportEmail: "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(strings.ToLower(msg.Subject), "new sign-in") {
		t.Errorf("Subject = %q, want it to read as a security alert", msg.Subject)
	}
	want := []string{"Chrome on macOS", "India", "203.0.113.9", "9 Aug 2026 at 01:19 IST"}
	for name, body := range map[string]string{"HTMLBody": msg.HTMLBody, "TextBody": msg.TextBody} {
		if body == "" {
			t.Fatalf("%s is empty", name)
		}
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s missing %q", name, w)
			}
		}
	}
}

// TestRenderNewDeviceLogin_UnknownCountry covers the geo-lookup miss: the
// alert must still send, because an unplaceable login is if anything more
// suspicious than a placeable one.
func TestRenderNewDeviceLogin_UnknownCountry(t *testing.T) {
	msg, err := RenderNewDeviceLogin("user@example.com", "noreply@mark8ly.com", NewDeviceLoginVars{
		Device:       "Unknown device",
		CountryName:  "an unknown location",
		IPAddress:    "",
		At:           "9 Aug 2026 at 01:19 UTC",
		SecureURL:    "https://admin.mark8ly.com/settings/security",
		SupportEmail: "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if msg.HTMLBody == "" || msg.TextBody == "" {
		t.Fatal("expected non-empty bodies")
	}
	if !strings.Contains(msg.TextBody, "an unknown location") {
		t.Errorf("TextBody missing the unknown-location fallback: %q", msg.TextBody)
	}
}

// TestSecurityTemplatesEscapeHTML guards the injection path: the device
// string is attacker-influenced (it derives from the User-Agent header),
// so it must never reach the HTML body unescaped.
func TestSecurityTemplatesEscapeHTML(t *testing.T) {
	msg, err := RenderNewDeviceLogin("user@example.com", "noreply@mark8ly.com", NewDeviceLoginVars{
		Device:       `<script>alert(1)</script>`,
		CountryName:  "India",
		IPAddress:    "203.0.113.9",
		At:           "9 Aug 2026",
		SecureURL:    "https://admin.mark8ly.com/settings/security",
		SupportEmail: "help@mark8ly.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(msg.HTMLBody, "<script>") {
		t.Error("HTMLBody contains an unescaped <script> tag from the device string")
	}
}

// TestRenderEmbedded_SecurityKeys asserts the two new keys are reachable
// through the loader's embedded dispatch, not just via their Render
// functions — that dispatch is what the send endpoint actually calls.
func TestRenderEmbedded_SecurityKeys(t *testing.T) {
	tests := []struct {
		key  string
		vars any
		want string
	}{
		{"login_otp", LoginOTPVars{Code: "112233", ExpiresIn: "5 minutes"}, "112233"},
		{"new_device_login", NewDeviceLoginVars{Device: "Safari on iOS", CountryName: "India"}, "Safari on iOS"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			msg, err := renderEmbedded(tt.key, "u@example.com", "noreply@mark8ly.com", "tenant-1", tt.vars)
			if err != nil {
				t.Fatalf("renderEmbedded(%s): %v", tt.key, err)
			}
			if !strings.Contains(msg.TextBody, tt.want) {
				t.Errorf("TextBody missing %q", tt.want)
			}
			if msg.TenantID != "tenant-1" {
				t.Errorf("TenantID = %q, want tenant-1", msg.TenantID)
			}
		})
	}
}

// TestRenderEmbedded_SecurityKeysRejectWrongVars asserts the type switch
// fails loudly rather than sending a half-rendered security email.
func TestRenderEmbedded_SecurityKeysRejectWrongVars(t *testing.T) {
	for _, key := range []string{"login_otp", "new_device_login"} {
		t.Run(key, func(t *testing.T) {
			if _, err := renderEmbedded(key, "u@example.com", "f@example.com", "", WelcomeVars{}); err == nil {
				t.Fatal("expected an error for mismatched vars type")
			}
		})
	}
}

// TestEmbeddedSeed_IncludesSecurityKeys asserts the new templates are
// seeded into email_templates so an operator can edit the copy without a
// redeploy, the same as every pre-existing key.
func TestEmbeddedSeed_IncludesSecurityKeys(t *testing.T) {
	seeded := make(map[string]seedRow)
	for _, s := range embeddedSeed() {
		seeded[s.key] = s
	}
	for _, key := range []string{"login_otp", "new_device_login"} {
		s, ok := seeded[key]
		if !ok {
			t.Errorf("embeddedSeed() missing key %q", key)
			continue
		}
		if s.subject == "" || s.html == "" || s.text == "" {
			t.Errorf("seed row %q has an empty field", key)
		}
	}
}
