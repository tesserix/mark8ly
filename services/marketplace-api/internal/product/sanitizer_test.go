package product_test

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/product"
)

func TestSanitizer_OWASPCorpus(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		banned []string // lowercased substrings that MUST NOT appear in the lowercased output
	}{
		{"script_tag", `<script>alert(1)</script>`, []string{"script", "alert"}},
		{"img_onerror", `<img src=x onerror=alert(1)>`, []string{"onerror", "alert", "<img"}},
		{"iframe", `<iframe src="javascript:alert(1)"></iframe>`, []string{"iframe", "javascript"}},
		{"svg_onload", `<svg onload=alert(1)>`, []string{"onload", "alert"}},
		{"a_javascript_href", `<a href="javascript:alert(1)">x</a>`, []string{"javascript"}},
		{"meta_refresh", `<meta http-equiv="refresh" content="0;url=http://evil">`, []string{"<meta", "refresh"}},
		{"style_expression", `<style>body{background:expression(alert(1))}</style>`, []string{"<style", "expression"}},
		{"object_tag", `<object data="evil.swf"></object>`, []string{"<object"}},
		{"embed_tag", `<embed src="evil.swf">`, []string{"<embed"}},
		{"form_tag", `<form action="evil"><input type=text></form>`, []string{"<form", "<input"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := product.Sanitize(tc.input)
			lo := strings.ToLower(out)
			for _, banned := range tc.banned {
				if strings.Contains(lo, banned) {
					t.Errorf("output %q contained banned token %q", out, banned)
				}
			}
		})
	}
}

func TestSanitizer_PlainText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// The sugarcoat report: v1 stored "&amp;" and the storefront showed it.
		{"ampersand", "Machine cut, Bubble free & Zero residue", "Machine cut, Bubble free & Zero residue"},
		{"quotes_and_angles", `Size "L" < 2kg > 1kg, it's`, `Size "L" < 2kg > 1kg, it's`},
		{"newlines_kept", "Line one\n\nLine two", "Line one\n\nLine two"},
		{"tags_stripped_text_kept", "<p>Hello <strong>world</strong></p>", "Hello world"},
		{"link_text_kept", `<a href="https://example.com">click</a>`, "click"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := product.Sanitize(tc.in)
			if got != tc.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Re-saving what the edit form shows must not change it.
			if again := product.Sanitize(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestSanitizer_EmptyInput(t *testing.T) {
	if product.Sanitize("") != "" {
		t.Errorf("expected empty output for empty input")
	}
}

func TestSanitizer_PolicyVersion(t *testing.T) {
	if product.SanitizerPolicyVersion != 2 {
		t.Fatalf("SanitizerPolicyVersion must stay 2 until an accompanying re-sanitize migration ships (spec §14.14); got %d", product.SanitizerPolicyVersion)
	}
}
