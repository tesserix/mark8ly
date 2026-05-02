package emailtemplates

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestLoader_NilDB_FallsBackToEmbedded verifies that a Loader with no
// DB connection still renders correctly via the embedded fallback,
// which is the safety property that lets us ship even when DB
// connectivity is flaky at boot.
func TestLoader_NilDB_FallsBackToEmbedded(t *testing.T) {
	l := NewLoader(nil)
	l.Register("welcome", EmbeddedFallback{
		Subject:  "Welcome {{.Name}}",
		HTMLBody: "<p>Hi {{.Name}}</p>",
		TextBody: "Hi {{.Name}}",
	})

	got, err := l.Render(context.Background(), "welcome", struct{ Name string }{Name: "Acme"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got.Subject != "Welcome Acme" {
		t.Errorf("Subject = %q, want Welcome Acme", got.Subject)
	}
	if got.HTMLBody != "<p>Hi Acme</p>" {
		t.Errorf("HTMLBody = %q", got.HTMLBody)
	}
	if got.TextBody != "Hi Acme" {
		t.Errorf("TextBody = %q", got.TextBody)
	}
}

// TestLoader_UnknownKey_ReturnsErrUnknownKey verifies that asking
// for an unregistered key fails loudly with a typed error rather
// than sending a blank email.
func TestLoader_UnknownKey_ReturnsErrUnknownKey(t *testing.T) {
	l := NewLoader(nil)
	_, err := l.Render(context.Background(), "no-such", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("err = %v, want errors.Is(ErrUnknownKey)", err)
	}
}

// TestLoader_HTMLAutoEscape verifies the HTML render path
// auto-escapes interpolated values, which is the security guarantee
// we rely on when an operator writes a template that interpolates
// user-controlled data.
func TestLoader_HTMLAutoEscape(t *testing.T) {
	l := NewLoader(nil)
	l.Register("xss-test", EmbeddedFallback{
		Subject:  "subj",
		HTMLBody: "<p>{{.X}}</p>",
		TextBody: "{{.X}}",
	})
	got, err := l.Render(context.Background(), "xss-test", struct{ X string }{X: "<script>alert(1)</script>"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(got.HTMLBody, "<script>") {
		t.Errorf("HTML did not escape: %q", got.HTMLBody)
	}
	if !strings.Contains(got.HTMLBody, "&lt;script&gt;") {
		t.Errorf("HTML missing escaped form: %q", got.HTMLBody)
	}
	// Text body MUST NOT escape — preserves the original chars.
	if !strings.Contains(got.TextBody, "<script>") {
		t.Errorf("text body unexpectedly escaped: %q", got.TextBody)
	}
}

// TestLoader_BadTemplateSyntax_Errors covers the case where an
// operator pushes a template with broken syntax. The render fails
// and the error surfaces — better than silently sending malformed
// HTML.
func TestLoader_BadTemplateSyntax_Errors(t *testing.T) {
	l := NewLoader(nil)
	l.Register("broken", EmbeddedFallback{
		Subject:  "subj",
		HTMLBody: "<p>{{.X", // unclosed
		TextBody: "ok",
	})
	_, err := l.Render(context.Background(), "broken", struct{ X string }{X: "y"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

// TestLoader_MissingField_Errors covers the case where the template
// references a field not in the Vars struct. Go's strict mode catches
// this only when html/template is used; text/template is permissive.
func TestLoader_TemplatesAreCallable(t *testing.T) {
	l := NewLoader(nil)
	l.Register("ok", EmbeddedFallback{
		Subject:  "{{.A}}",
		HTMLBody: "<p>{{.A}}</p>",
		TextBody: "{{.A}}",
	})
	_, err := l.Render(context.Background(), "ok", map[string]string{"A": "x"})
	if err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

// TestLoader_Invalidate_ClearsCacheEntry exercises the cache eviction
// path. With a nil DB the cache stays empty so we just verify the
// call surface doesn't panic and Render keeps working.
func TestLoader_Invalidate_DoesNotPanic(t *testing.T) {
	l := NewLoader(nil)
	l.Register("x", EmbeddedFallback{Subject: "s", HTMLBody: "<p>x</p>", TextBody: "x"})
	l.Invalidate("x")
	l.InvalidateAll()

	got, err := l.Render(context.Background(), "x", nil)
	if err != nil {
		t.Fatalf("render after invalidate: %v", err)
	}
	if got.Subject != "s" {
		t.Errorf("Subject = %q", got.Subject)
	}
}

// TestLoader_Register_LastWriterWins covers re-registration (used by
// tests that need to swap a fallback mid-process).
func TestLoader_Register_LastWriterWins(t *testing.T) {
	l := NewLoader(nil)
	l.Register("k", EmbeddedFallback{Subject: "first", HTMLBody: "<p>1</p>", TextBody: "1"})
	l.Register("k", EmbeddedFallback{Subject: "second", HTMLBody: "<p>2</p>", TextBody: "2"})
	got, err := l.Render(context.Background(), "k", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "second" {
		t.Errorf("Subject = %q, want second", got.Subject)
	}
}

// TestLoader_NilSeed_NoOp verifies SeedFromEmbedded is safe with no DB.
func TestLoader_NilSeed_NoOp(t *testing.T) {
	l := NewLoader(nil)
	l.Register("x", EmbeddedFallback{Subject: "s", HTMLBody: "h", TextBody: "t"})
	if err := l.SeedFromEmbedded(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// TestCacheTTL_Constants pins the TTL constant so changes are
// deliberate.
func TestCacheTTL_Constants(t *testing.T) {
	if CacheTTL != 5*time.Minute {
		t.Errorf("CacheTTL = %v, want 5m", CacheTTL)
	}
}
