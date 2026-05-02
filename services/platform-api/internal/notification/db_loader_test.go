package notification

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestLoader_NilDB_FallsBackToEmbedded verifies that a Loader with no
// DB connection still renders correctly via the embedded fallback. This
// is the safety property that lets us ship even when DB connectivity is
// flaky at boot — emails keep flowing.
func TestLoader_NilDB_FallsBackToEmbedded(t *testing.T) {
	l := NewLoader(nil)

	cases := []struct {
		name string
		key  string
		vars any
		want string // substring expected in HTMLBody
	}{
		{
			name: "welcome",
			key:  "welcome",
			vars: WelcomeVars{
				BusinessName:  "Acme",
				OwnerName:     "Pat",
				AdminURL:      "https://acme-admin.mark8ly.com",
				StorefrontURL: "https://acme.mark8ly.com",
				SupportEmail:  "help@mark8ly.com",
			},
			want: "Acme",
		},
		{
			name: "email_verification",
			key:  "email_verification",
			vars: EmailVerificationVars{
				BusinessName: "",
				VerifyURL:    "https://onboarding.mark8ly.com/onboarding/verify?token=tok-abc",
				ExpiresIn:    "24 hours",
				SupportEmail: "help@mark8ly.com",
			},
			want: "tok-abc",
		},
		{
			name: "invitation",
			key:  "invitation",
			vars: InvitationVars{
				TenantName:   "Acme Co",
				Role:         "admin",
				Inviter:      "Pat",
				AcceptURL:    "https://acme-admin.mark8ly.com/accept-invite?token=inv-1",
				ExpiresIn:    "72 hours",
				SupportEmail: "help@mark8ly.com",
			},
			want: "Acme Co",
		},
		{
			name: "password_reset",
			key:  "password_reset",
			vars: PasswordResetVars{
				ResetURL:     "https://admin.mark8ly.com/reset-password?oobCode=oob-1",
				ExpiresIn:    "1 hour",
				SupportEmail: "help@mark8ly.com",
			},
			want: "oob-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := l.Render(context.Background(), tc.key, "user@example.com", "noreply@mark8ly.com", "tenant-1", tc.vars)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if msg.To != "user@example.com" {
				t.Errorf("To = %q, want user@example.com", msg.To)
			}
			if msg.From != "noreply@mark8ly.com" {
				t.Errorf("From = %q, want noreply@mark8ly.com", msg.From)
			}
			if msg.TenantID != "tenant-1" {
				t.Errorf("TenantID = %q, want tenant-1 (loader should forward through fallback)", msg.TenantID)
			}
			if !strings.Contains(msg.HTMLBody, tc.want) {
				t.Errorf("HTMLBody missing %q (template did not interpolate vars correctly)", tc.want)
			}
			if msg.TextBody == "" {
				t.Error("TextBody empty")
			}
			if msg.Subject == "" {
				t.Error("Subject empty")
			}
		})
	}
}

// TestLoader_UnknownKey_ReturnsError verifies that asking for a key
// that has neither a DB row nor an embedded fallback fails loudly
// instead of sending a blank email.
func TestLoader_UnknownKey_ReturnsError(t *testing.T) {
	l := NewLoader(nil)
	_, err := l.Render(context.Background(), "no-such-template", "user@example.com", "noreply@mark8ly.com", "", WelcomeVars{})
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

// TestLoader_WrongVarsType_ReturnsError verifies that handing the
// wrong Vars type (e.g. WelcomeVars to invitation) fails the type
// assertion in the embedded fallback rather than panicking.
func TestLoader_WrongVarsType_ReturnsError(t *testing.T) {
	l := NewLoader(nil)
	_, err := l.Render(context.Background(), "invitation", "user@example.com", "noreply@mark8ly.com", "", WelcomeVars{
		BusinessName: "Acme",
	})
	if err == nil {
		t.Fatal("expected type-assertion error, got nil")
	}
}

// TestLoader_MissingRecipient_Errors verifies that the boundary check
// catches an empty To address before any rendering happens.
func TestLoader_MissingRecipient_Errors(t *testing.T) {
	l := NewLoader(nil)
	_, err := l.Render(context.Background(), "welcome", "", "noreply@mark8ly.com", "", WelcomeVars{})
	if err == nil {
		t.Fatal("expected ErrNoRecipient, got nil")
	}
}

// TestLoader_NilSeed_NoOp verifies SeedFromEmbedded is safe to call
// with no DB. Useful for tests + boot races where the DB isn't ready.
func TestLoader_NilSeed_NoOp(t *testing.T) {
	l := NewLoader(nil)
	if err := l.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed nil: %v", err)
	}
}

// TestLoader_Invalidate_ClearsCacheEntry exercises the cache eviction
// path used by the /internal/templates/refresh endpoint. We can't
// observe the cache directly so we test the public surface: an
// Invalidate call should cause a subsequent Render to re-traverse the
// load path. With a nil DB the load is a no-op, so we just verify the
// call doesn't panic and the second render still produces output.
func TestLoader_Invalidate_DoesNotPanic(t *testing.T) {
	l := NewLoader(nil)
	l.Invalidate("welcome")
	l.InvalidateAll()

	msg, err := l.Render(context.Background(), "welcome", "u@e.com", "n@m.com", "", WelcomeVars{
		BusinessName: "Acme", AdminURL: "x", StorefrontURL: "y", SupportEmail: "z",
	})
	if err != nil {
		t.Fatalf("render after invalidate: %v", err)
	}
	if msg.HTMLBody == "" {
		t.Error("expected non-empty body after invalidate")
	}
}

// TestRenderInline_HTMLAutoEscape verifies the HTML render path
// auto-escapes interpolated values, which is the security guarantee
// we rely on when an operator writes a template that interpolates
// user-controlled data (e.g. business names with HTML chars).
func TestRenderInline_HTMLAutoEscape(t *testing.T) {
	out, err := renderInline("html:test", `<p>{{.X}}</p>`, struct{ X string }{X: "<script>alert(1)</script>"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("HTML render did not escape: %q", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("HTML render did not produce escaped form: %q", out)
	}
}

// TestRenderInline_TextNoEscape verifies the text render path does
// NOT escape — plain-text bodies should preserve the original chars.
func TestRenderInline_TextNoEscape(t *testing.T) {
	out, err := renderInline("text:test", `> {{.X}}`, struct{ X string }{X: "ok & fine"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "& fine") {
		t.Errorf("text render escaped ampersand unexpectedly: %q", out)
	}
}

// TestEmbeddedSeed_AllKeysPresent ensures every Render* function has a
// corresponding seed row. If a future template is added without a seed
// entry the catch is at boot (seed insert) instead of mid-send.
func TestEmbeddedSeed_AllKeysPresent(t *testing.T) {
	want := []string{"welcome", "email_verification", "invitation", "password_reset"}
	seeds := embeddedSeed()
	got := map[string]bool{}
	for _, s := range seeds {
		got[s.key] = true
		if s.subject == "" {
			t.Errorf("seed %q: empty subject", s.key)
		}
		if s.html == "" {
			t.Errorf("seed %q: empty html", s.key)
		}
		if s.text == "" {
			t.Errorf("seed %q: empty text", s.key)
		}
		if s.varsJSON == "" || s.varsJSON[0] != '[' {
			t.Errorf("seed %q: varsJSON not a JSON array: %q", s.key, s.varsJSON)
		}
	}
	for _, k := range want {
		if !got[k] {
			t.Errorf("seed missing key %q", k)
		}
	}
}

// TestCacheTTL_Constants pins the TTL constants so changes are
// deliberate. CI failure here forces a maintainer to rethink before
// shipping a much shorter or longer TTL.
func TestCacheTTL_Constants(t *testing.T) {
	if CacheTTL != 5*time.Minute {
		t.Errorf("CacheTTL = %v, want 5m (change is deliberate? update test)", CacheTTL)
	}
}
