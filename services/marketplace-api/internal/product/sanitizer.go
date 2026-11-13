// services/marketplace-api/internal/product/sanitizer.go
package product

import (
	"html"

	"github.com/microcosm-cc/bluemonday"
)

// SanitizerPolicyVersion pins the description policy. Incrementing this is
// append-only and requires an accompanying re-sanitization migration that
// re-runs every stored product description through the new policy. See
// spec §14.14. Don't bump this without a migration in the same PR.
//
// v2 (migration 000143): descriptions are plain text. Every editor is a
// plain textarea and every reader renders text, so v1's stored HTML showed
// buyers a literal "&amp;" for every "&" — and grew to "&amp;amp;" each
// time the merchant re-saved the escaped text from the edit form.
const SanitizerPolicyVersion = 2

var policy = bluemonday.StrictPolicy()

// Sanitize returns the input as plain text: every tag is stripped (script
// and style along with their content) and entities are decoded. Readers
// MUST render the result as text, never as HTML; React text nodes and
// html/template both do. Empty input returns empty string. Applied at
// write time only; stored text is never re-sanitized on read.
func Sanitize(in string) string {
	if in == "" {
		return ""
	}
	return html.UnescapeString(policy.Sanitize(in))
}
