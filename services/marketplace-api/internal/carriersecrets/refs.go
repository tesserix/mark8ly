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
//   - "bao://kv/mark8ly/marketplace-api/tenants/{TENANTID}/{DOMAIN}/{PROVIDER}/{FIELD}"
//     OpenBao KV v2 logical path reference; carries no version so rotations
//     return the identical reference.
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
	// BaoRefPrefix marks an OpenBao KV v2 reference. The path that follows is
	// the logical KV path WITHOUT the `data/` or `metadata/` infix — those are
	// KV v2 API details the client adds, not part of the stored reference.
	//
	// Deliberately carries NO version: KV v2 writes a new version at the same
	// path, so a rotation returns this identical reference. Encoding a version
	// here would turn every rotation into a reference rewrite across the DB.
	BaoRefPrefix = "bao://"
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

// IsBaoRef reports whether r is an OpenBao reference.
func IsBaoRef(r string) bool { return strings.HasPrefix(r, BaoRefPrefix) }

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

// baoPathPrefix is the namespace-scoped root for every carrier credential.
// The estate convention is that the first path segment is the Kubernetes
// namespace (compare kv/data/homechef/*, kv/data/devai/devai-api/*), and
// environments are separated by cluster — so there is no env segment.
const baoPathPrefix = "kv/mark8ly/marketplace-api/tenants"

// BaoPath returns the logical KV path for a scope. Segments are sanitised
// with the same rules as the GCP secret ID so a stray '/' or '..' in a
// scope component cannot escape the tenant's subtree.
func BaoPath(s Scope) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		baoPathPrefix,
		sanitizeSegment(s.TenantID),
		sanitizeSegment(strings.ToLower(s.Domain)),
		sanitizeSegment(strings.ToLower(s.Provider)),
		sanitizeSegment(strings.ToLower(s.Field)),
	)
}

// FormatBaoReference returns the canonical reference persisted to the DB.
func FormatBaoReference(s Scope) string { return BaoRefPrefix + BaoPath(s) }

// ParseBaoReference extracts the logical KV path from a bao:// reference.
func ParseBaoReference(ref string) (path string, ok bool) {
	if !IsBaoRef(ref) {
		return "", false
	}
	return strings.TrimPrefix(ref, BaoRefPrefix), true
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
