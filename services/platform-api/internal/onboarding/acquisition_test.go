package onboarding

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeAcquisition_KeepsAllowlistedFieldsOnly(t *testing.T) {
	raw := json.RawMessage(`{
		"first": {
			"utm_source": " newsletter ",
			"utm_medium": "email",
			"utm_campaign": "spring",
			"utm_content": "hero",
			"utm_term": "makers",
			"landing_path": "/ecommerce-for-makers?utm_source=newsletter&token=abc#top",
			"referrer": "https://news.example.org/issue/12?session=secret",
			"captured_at": "2026-10-05T10:00:00+10:00",
			"password": "hunter2",
			"email": "a@b.c"
		},
		"last": null,
		"raw_query": "utm_source=x&token=y"
	}`)

	out, ok := SanitizeAcquisition(raw)
	if !ok {
		t.Fatal("expected a sanitised record")
	}
	var got Acquisition
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.First == nil || got.Last != nil {
		t.Fatalf("expected first only, got %+v", got)
	}
	want := Touch{
		Source:      "newsletter",
		Medium:      "email",
		Campaign:    "spring",
		Content:     "hero",
		Term:        "makers",
		LandingPath: "/ecommerce-for-makers",
		Referrer:    "https://news.example.org/issue/12",
		CapturedAt:  "2026-10-05T00:00:00Z",
	}
	if *got.First != want {
		t.Fatalf("first touch\n got %+v\nwant %+v", *got.First, want)
	}
	for _, leak := range []string{"token", "secret", "hunter2", "a@b.c", "raw_query"} {
		if strings.Contains(string(out), leak) {
			t.Fatalf("sanitised record leaks %q: %s", leak, out)
		}
	}
}

func TestSanitizeAcquisition_DropsWhatItCannotTrust(t *testing.T) {
	cases := map[string]string{
		"not json":              `{"first": `,
		"null":                  `null`,
		"empty object":          `{}`,
		"empty touches":         `{"first": {}, "last": {"utm_source": "   "}}`,
		"relative landing path": `{"first": {"landing_path": "onboarding"}}`,
		"protocol-relative":     `{"first": {"landing_path": "//evil.example/x"}}`,
		"javascript referrer":   `{"first": {"referrer": "javascript:alert(1)"}}`,
		"bad timestamp only":    `{"first": {"captured_at": "yesterday"}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			out, ok := SanitizeAcquisition(json.RawMessage(raw))
			if ok || out != nil {
				t.Fatalf("expected nothing to survive, got ok=%v out=%s", ok, out)
			}
		})
	}
}

func TestSanitizeAcquisition_CapsLengthAndStripsControlChars(t *testing.T) {
	long := strings.Repeat("a", maxTouchFieldLen+50)
	raw := json.RawMessage(`{"last": {"utm_source": "` + long + `", "utm_campaign": "spr\u0000ing\n"}}`)
	out, ok := SanitizeAcquisition(raw)
	if !ok {
		t.Fatal("expected a record")
	}
	var got Acquisition
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Last == nil {
		t.Fatal("expected last touch")
	}
	if len(got.Last.Source) != maxTouchFieldLen {
		t.Fatalf("source length %d, want %d", len(got.Last.Source), maxTouchFieldLen)
	}
	if got.Last.Campaign != "spring" {
		t.Fatalf("campaign %q, want control characters stripped", got.Last.Campaign)
	}
}

func TestClassifier_ByEmail(t *testing.T) {
	c := NewClassifier(
		[]string{" Tesserix.app ", "mark8ly.com", ""},
		[]string{"Demo@Shop.Example.COM", "bondi@mark8ly.com"},
		nil,
	)
	cases := map[string]string{
		"founder@gmail.com":     ClassificationExternal,
		"e2e-abc@example.com":   ClassificationTest,
		"x@sub.example.org":     ClassificationExternal, // only the bare reserved domains
		"x@store.test":          ClassificationTest,
		"x@dev.localhost":       ClassificationTest,
		"ops@tesserix.app":      ClassificationInternal,
		"OPS@MARK8LY.COM":       ClassificationInternal,
		"demo@shop.example.com": ClassificationDemo, // demo wins over test domain
		"bondi@mark8ly.com":     ClassificationDemo, // demo wins over internal
		"no-at-sign":            ClassificationExternal,
		"":                      ClassificationExternal,
	}
	for email, want := range cases {
		if got := c.ByEmail(email); got != want {
			t.Errorf("ByEmail(%q) = %q, want %q", email, got, want)
		}
	}
}

func TestClassifier_ZeroValueStillSeparatesFixtures(t *testing.T) {
	var c Classifier
	if got := c.ByEmail("e2e@example.com"); got != ClassificationTest {
		t.Fatalf("zero classifier: got %q for reserved domain", got)
	}
	if got := c.ByEmail("someone@real-shop.com"); got != ClassificationExternal {
		t.Fatalf("zero classifier: got %q for a real domain", got)
	}
}

func TestClassifier_AtCompletion(t *testing.T) {
	c := NewClassifier(nil, nil, []string{"the-bondi-store", " ACME-DEMO "})
	cases := []struct{ current, slug, want string }{
		{ClassificationExternal, "the-bondi-store", ClassificationDemo},
		{ClassificationInternal, "Acme-Demo", ClassificationDemo},
		{ClassificationExternal, "real-shop", ClassificationExternal},
		{ClassificationTest, "real-shop", ClassificationTest},
		{"", "real-shop", ClassificationExternal},
	}
	for _, tc := range cases {
		if got := c.AtCompletion(tc.current, tc.slug); got != tc.want {
			t.Errorf("AtCompletion(%q, %q) = %q, want %q", tc.current, tc.slug, got, tc.want)
		}
	}
}

func TestIsClassification(t *testing.T) {
	for _, ok := range []string{"external", "internal", "test", "demo"} {
		if !IsClassification(ok) {
			t.Errorf("%q should be a classification", ok)
		}
	}
	for _, bad := range []string{"", "External", "all", "'; DROP TABLE"} {
		if IsClassification(bad) {
			t.Errorf("%q should not be a classification", bad)
		}
	}
}
