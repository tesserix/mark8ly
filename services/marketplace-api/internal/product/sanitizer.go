// services/marketplace-api/internal/product/sanitizer.go
package product

import "github.com/microcosm-cc/bluemonday"

// SanitizerPolicyVersion pins the bluemonday policy. Incrementing this is
// append-only and requires an accompanying re-sanitization migration that
// re-runs every stored product description through the new policy. See
// spec §14.14. Don't bump this without a migration in the same PR.
const SanitizerPolicyVersion = 1

var policy = policyV1()

func policyV1() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "u",
		"ul", "ol", "li",
		"h2", "h3", "h4",
		"blockquote")
	p.AllowAttrs("href").OnElements("a")
	p.RequireNoFollowOnLinks(true)
	p.AllowURLSchemes("http", "https", "mailto")
	return p
}

// Sanitize returns a safe rendering of the input HTML per the pinned
// policy. Empty input returns empty string. Sanitize is only applied to
// user-authored rich text at write time; stored HTML is never
// re-sanitized on read.
func Sanitize(in string) string {
	if in == "" {
		return ""
	}
	return policy.Sanitize(in)
}
