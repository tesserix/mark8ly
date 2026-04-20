// Package carriersecrets resolves per-tenant carrier credentials
// (Delhivery, Razorpay, TaxJar, etc.) out of GCP Secret Manager with
// a fallback to envelope-encrypted ciphertext stored inline. Callers
// persist the opaque "reference" string returned by Put into the
// shipping_carrier_configs.api_key_encrypted column (or the equivalent
// payment/tax column); Get transparently resolves either form.
//
// Reference formats recognised by the Store:
//
//   - "gsm://projects/{PROJECT}/secrets/{NAME}"
//     fully-qualified GCP Secret Manager reference; always read at
//     /versions/latest.
//
//   - "noop:..." / "aes:..."
//     legacy inline ciphertext from crypto.Encryptor. Kept readable
//     so existing rows keep working until the next save migrates
//     them. HybridStore.MaybeRewrap performs that lazy migration.
package carriersecrets

import (
	"fmt"
	"strings"
)

// Reference prefixes. Every value persisted to the DB starts with
// exactly one of these.
const (
	// GSMRefPrefix marks a GCP Secret Manager resource reference.
	GSMRefPrefix = "gsm://"
	// NoopRefPrefix marks a base64-wrapped dev-mode "ciphertext".
	NoopRefPrefix = "noop:"
	// AESRefPrefix marks an AES-256-GCM ciphertext.
	AESRefPrefix = "aes:"
)

// IsGSMRef reports whether r is a GCP SM reference — i.e. a value
// this package produced on a previous Put against a GCPStore or
// HybridStore in gcpsm mode.
func IsGSMRef(r string) bool { return strings.HasPrefix(r, GSMRefPrefix) }

// IsInlineRef reports whether r is a legacy inline-encrypted value
// (noop: / aes:) that a crypto.Encryptor can decode.
func IsInlineRef(r string) bool {
	return strings.HasPrefix(r, NoopRefPrefix) || strings.HasPrefix(r, AESRefPrefix)
}

// SecretName returns the Secret Manager secret ID for a scope:
//
//	{prefix}-{tenant}-{domain}-{provider}-{field}
//
// GCP Secret Manager caps the secret ID at 255 characters and
// restricts it to `[A-Za-z0-9_-]`. Our layout stays well under the
// limit: a prefix of <= 20 chars + a UUID (36) + three short
// segments (~40 chars combined) is ~96 chars for any realistic
// tenant.
func SecretName(prefix string, s Scope) string {
	tenant := sanitizeSegment(s.TenantID)
	domain := sanitizeSegment(strings.ToLower(s.Domain))
	provider := sanitizeSegment(strings.ToLower(s.Provider))
	field := sanitizeSegment(strings.ToLower(s.Field))
	return fmt.Sprintf("%s-%s-%s-%s-%s", prefix, tenant, domain, provider, field)
}

// SecretResource returns the fully-qualified GCP SM resource for a
// scope: projects/{project}/secrets/{name}. This is the value the
// underlying Google SDK accepts as-is.
func SecretResource(projectID, prefix string, s Scope) string {
	return fmt.Sprintf("projects/%s/secrets/%s", projectID, SecretName(prefix, s))
}

// FormatReference returns the canonical "gsm://..." reference string
// we persist to the DB for a scope.
func FormatReference(projectID, prefix string, s Scope) string {
	return GSMRefPrefix + SecretResource(projectID, prefix, s)
}

// ParseReference extracts the GCP SM resource (projects/…/secrets/…)
// from a "gsm://" reference. Returns ("", false) for anything else.
func ParseReference(ref string) (resource string, ok bool) {
	if !IsGSMRef(ref) {
		return "", false
	}
	return strings.TrimPrefix(ref, GSMRefPrefix), true
}

// sanitizeSegment reduces a Scope segment to the character set GCP
// Secret Manager accepts: letters, digits, '_' and '-'. Anything else
// becomes '_' so a stray dot/slash never produces an InvalidArgument
// from the API.
func sanitizeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
