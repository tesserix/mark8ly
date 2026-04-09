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

func TestSanitizer_PreservesAllowedTags(t *testing.T) {
	in := `<p>Hello <strong>world</strong> <em>now</em></p><ul><li>one</li></ul>`
	out := product.Sanitize(in)
	for _, tag := range []string{"<p>", "<strong>", "<em>", "<ul>", "<li>"} {
		if !strings.Contains(out, tag) {
			t.Errorf("expected %q preserved in %q", tag, out)
		}
	}
}

func TestSanitizer_ForcesNofollowOnLinks(t *testing.T) {
	in := `<a href="https://example.com" target="_blank">click</a>`
	out := product.Sanitize(in)
	if !strings.Contains(out, `rel="nofollow"`) {
		t.Errorf("expected rel=nofollow on links; got %q", out)
	}
	if strings.Contains(out, "target") {
		t.Errorf("expected target attribute stripped; got %q", out)
	}
}

func TestSanitizer_EmptyInput(t *testing.T) {
	if product.Sanitize("") != "" {
		t.Errorf("expected empty output for empty input")
	}
}

func TestSanitizer_PolicyVersion(t *testing.T) {
	if product.SanitizerPolicyVersion != 1 {
		t.Fatalf("SanitizerPolicyVersion must stay 1 until an accompanying re-sanitize migration ships (spec §14.14); got %d", product.SanitizerPolicyVersion)
	}
}
