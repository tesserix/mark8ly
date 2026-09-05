package email

import (
	"strings"
	"testing"
)

const platformFrom = "noreply@tesserix.app"

func TestDeriveLocalPart(t *testing.T) {
	cases := []struct{ slug, want string }{
		{"nadias-ceramics", "nadias-ceramics"},
		{"Nadias-Ceramics", "nadias-ceramics"},
		{"acme", "acme"},
		{"store123", "store123"},

		// Normalisation: anything outside [a-z0-9-] collapses to one dash
		// and never leaks to the edges.
		{"Nadia's Ceramics", "nadia-s-ceramics"},
		{"  spaced  out  ", "spaced-out"},
		{"--leading--and--trailing--", "leading-and-trailing"},
		{"dots.and_underscores", "dots-and-underscores"},

		// Nothing usable survives — caller keeps the platform address.
		{"", ""},
		{"   ", ""},
		{"---", ""},
		{"日本語", ""},
		{"!!!", ""},

		// A non-ASCII slug must not reach the wire as-is; SMTPUTF8 is not
		// something either provider is configured for.
		{"café", "caf"},

		// Reserved local parts are escaped, so no merchant can claim an
		// address that reads as the platform speaking.
		{"support", "store-support"},
		{"billing", "store-billing"},
		{"noreply", "store-noreply"},
		{"security", "store-security"},
		{"mark8ly", "store-mark8ly"},
		{"Support", "store-support"},

		// ...and the escape space is itself escaped, so the mapping stays
		// injective: "support" and "store-support" cannot collide.
		{"store-support", "store-store-support"},
		{"store-anything", "store-store-anything"},
	}
	for _, c := range cases {
		if got := DeriveLocalPart(c.slug); got != c.want {
			t.Errorf("DeriveLocalPart(%q) = %q, want %q", c.slug, got, c.want)
		}
	}
}

// TestDeriveLocalPart_Injective is the collision rule stated as a
// property: distinct slugs must never produce the same local part, or
// two stores would share one sender address.
func TestDeriveLocalPart_Injective(t *testing.T) {
	slugs := []string{
		"support", "store-support", "store-store-support",
		"billing", "store-billing", "nadias-ceramics", "store", "store-store",
	}
	seen := map[string]string{}
	for _, slug := range slugs {
		got := DeriveLocalPart(slug)
		if prev, dup := seen[got]; dup {
			t.Errorf("collision: %q and %q both derive %q", prev, slug, got)
		}
		seen[got] = slug
	}
}

func TestDeriveLocalPart_RespectsRFCLength(t *testing.T) {
	got := DeriveLocalPart(strings.Repeat("a", 200))
	if len(got) > maxLocalPartLen {
		t.Errorf("local part %d chars, want <= %d", len(got), maxLocalPartLen)
	}
	if strings.HasSuffix(got, "-") || strings.HasPrefix(got, "-") {
		t.Errorf("truncation left a dangling separator: %q", got)
	}
}

// TestSafeDisplayName_Hostile is the phishing surface. The store name is
// merchant-controlled text rendered in a stranger's inbox.
func TestSafeDisplayName_Hostile(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"honest name passes through", "Nadia's Ceramics", "Nadia's Ceramics"},
		{"unicode name survives", "Café Noir", "Café Noir"},

		// The #718 headline case.
		{"platform support impersonation", "Mark8ly Support", PlatformDisplayName},
		{"platform billing impersonation", "Mark8ly Billing", PlatformDisplayName},
		{"platform name alone", "mark8ly", PlatformDisplayName},
		{"platform name spaced out", "M a r k 8 l y  S e c u r i t y", PlatformDisplayName},
		{"platform name punctuated", "Mark-8-ly Support", PlatformDisplayName},
		{"sibling brand", "Tesserix Ops", PlatformDisplayName},

		// Address-shaped names: a client rendering these can show a
		// second, forged address next to the real one.
		{"embedded address", "Nadia <security@mark8ly.com>", PlatformDisplayName},
		{"bare address", "security@example.com", PlatformDisplayName},
		{"quoted spoof", `"Support" <a@b.com>`, PlatformDisplayName},
		{"comment syntax", "Nadia (via mark8ly)", PlatformDisplayName},

		// Header injection at the name layer.
		{"crlf injection", "Nadia\r\nBcc: victim@example.com", PlatformDisplayName},
		{"newline only", "Nadia\nX-Spoof: 1", "NadiaX-Spoof 1"},
		{"comma survives", "Acme, Inc.", "Acme Inc."},
		{"nul byte", "Nadia\x00", "Nadia"},

		// The #718 acceptance criterion: an unnamed store keeps the
		// "Mark8ly" body-copy fallback.
		{"empty name", "", PlatformDisplayName},
		{"whitespace only", "   \t  ", PlatformDisplayName},
		{"specials only", "<>@,;", PlatformDisplayName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SafeDisplayName(c.input)
			if got != c.want {
				t.Fatalf("SafeDisplayName(%q) = %q, want %q", c.input, got, c.want)
			}
			// Whatever survives must be transport-safe, unconditionally.
			if strings.ContainsAny(got, "\r\n\x00") {
				t.Errorf("result carries a control character: %q", got)
			}
			if strings.ContainsAny(got, fatalNameChars+strippableNameChars) {
				t.Errorf("result carries an RFC 5322 special: %q", got)
			}
		})
	}
}

func TestSafeDisplayName_Truncates(t *testing.T) {
	got := SafeDisplayName(strings.Repeat("ü", 200))
	if n := len([]rune(got)); n > maxDisplayNameRunes {
		t.Errorf("display name %d runes, want <= %d", n, maxDisplayNameRunes)
	}
}

func TestStoreIdentity(t *testing.T) {
	got := StoreIdentity(platformFrom, StoreSender{
		Name:         "Nadia's Ceramics",
		Slug:         "nadias-ceramics",
		ContactEmail: "hello@nadiasceramics.com",
	})

	// The local part is derived; the DOMAIN comes from the configured
	// platform address and is never hardcoded, so this keeps working
	// after tesserix/tesserix-k8s#1011 moves the sending domain.
	if got.From != "nadias-ceramics@tesserix.app" {
		t.Errorf("From = %q", got.From)
	}
	if got.FromName != "Nadia's Ceramics" {
		t.Errorf("FromName = %q", got.FromName)
	}
	if got.ReplyTo != "hello@nadiasceramics.com" {
		t.Errorf("ReplyTo = %q", got.ReplyTo)
	}
}

func TestStoreIdentity_FollowsConfiguredDomain(t *testing.T) {
	got := StoreIdentity("noreply@mail.mark8ly.com", StoreSender{
		Name: "Nadia's Ceramics", Slug: "nadias-ceramics",
	})
	if got.From != "nadias-ceramics@mail.mark8ly.com" {
		t.Errorf("From = %q; the domain must follow EMAIL_FROM", got.From)
	}
}

// TestStoreIdentity_Degrades pins that no input can produce a broken
// envelope: an empty Reply-To, a bare "@domain", or an empty From name.
func TestStoreIdentity_Degrades(t *testing.T) {
	cases := []struct {
		name         string
		in           StoreSender
		wantFrom     string
		wantFromName string
		wantReplyTo  string
	}{
		{
			name:         "no slug keeps the platform local part",
			in:           StoreSender{Name: "Nadia's Ceramics"},
			wantFrom:     platformFrom,
			wantFromName: "Nadia's Ceramics",
			wantReplyTo:  platformFrom,
		},
		{
			name:         "underivable slug keeps the platform local part",
			in:           StoreSender{Name: "Nadia's Ceramics", Slug: "日本語"},
			wantFrom:     platformFrom,
			wantFromName: "Nadia's Ceramics",
			wantReplyTo:  platformFrom,
		},
		{
			name:         "unnamed store falls back to Mark8ly",
			in:           StoreSender{Slug: "acme"},
			wantFrom:     "acme@tesserix.app",
			wantFromName: PlatformDisplayName,
			wantReplyTo:  platformFrom,
		},
		{
			name:         "hostile name falls back to Mark8ly",
			in:           StoreSender{Name: "Mark8ly Support", Slug: "acme"},
			wantFrom:     "acme@tesserix.app",
			wantFromName: PlatformDisplayName,
			wantReplyTo:  platformFrom,
		},
		{
			name:         "unroutable contact falls back to the platform address",
			in:           StoreSender{Name: "Acme", Slug: "acme", ContactEmail: "owner@acme.local"},
			wantFrom:     "acme@tesserix.app",
			wantFromName: "Acme",
			wantReplyTo:  platformFrom,
		},
		{
			name:         "malformed contact falls back to the platform address",
			in:           StoreSender{Name: "Acme", Slug: "acme", ContactEmail: "not-an-address"},
			wantFrom:     "acme@tesserix.app",
			wantFromName: "Acme",
			wantReplyTo:  platformFrom,
		},
		{
			name:         "reserved slug is escaped",
			in:           StoreSender{Name: "Support Co", Slug: "support"},
			wantFrom:     "store-support@tesserix.app",
			wantFromName: "Support Co",
			wantReplyTo:  platformFrom,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StoreIdentity(platformFrom, c.in)
			if got.From != c.wantFrom {
				t.Errorf("From = %q, want %q", got.From, c.wantFrom)
			}
			if got.FromName != c.wantFromName {
				t.Errorf("FromName = %q, want %q", got.FromName, c.wantFromName)
			}
			if got.ReplyTo != c.wantReplyTo {
				t.Errorf("ReplyTo = %q, want %q", got.ReplyTo, c.wantReplyTo)
			}
			if got.ReplyTo == "" {
				t.Error("ReplyTo is empty; a store identity must always offer a reply path")
			}
		})
	}
}

// TestStoreIdentity_AlwaysPassesTransportValidation is the belt-and-
// braces check: no merchant input may produce a Message the transport
// then refuses, which would drop a real order confirmation.
func TestStoreIdentity_AlwaysPassesTransportValidation(t *testing.T) {
	hostile := []StoreSender{
		{Name: "Mark8ly Support", Slug: "support", ContactEmail: "a@b.local"},
		{Name: "Nadia\r\nBcc: victim@example.com", Slug: "nadia\r\nx"},
		{Name: "<script>alert(1)</script>", Slug: strings.Repeat("z", 300)},
		{Name: "", Slug: "", ContactEmail: ""},
		{Name: "N", Slug: "-", ContactEmail: "a@b.com\r\nBcc: v@e.com"},
	}
	for _, s := range hostile {
		msg := Message{To: "buyer@example.com", Subject: "s", TextBody: "b"}
		StoreIdentity(platformFrom, s).Apply(&msg)
		if err := validate(msg); err != nil {
			t.Errorf("StoreIdentity(%+v) produced an unsendable message: %v", s, err)
		}
	}
}

// TestPlatformIdentity keeps mark8ly-to-merchant mail on the platform
// address and offers no reply path into an unattended noreply box.
func TestPlatformIdentity(t *testing.T) {
	got := PlatformIdentity(platformFrom, "Mark8ly Billing")
	if got.From != platformFrom {
		t.Errorf("From = %q", got.From)
	}
	if got.FromName != "Mark8ly Billing" {
		t.Errorf("FromName = %q", got.FromName)
	}
	if got.ReplyTo != "" {
		t.Errorf("ReplyTo = %q, want empty", got.ReplyTo)
	}
}
