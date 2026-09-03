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
//     them. ChainStore.MaybeRewrap performs that lazy migration.
//
// "gsm://" (GCP Secret Manager) references are recognised for
// classification (IsGSMRef) but no longer resolve: GCP Secret Manager was
// retired from this package in mark8ly#621. A stored gsm:// reference now
// means a row the mark8ly#621 backfill missed, not a live credential path.
package carriersecrets

import (
	"fmt"
	"strconv"
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

// IsGSMRef reports whether r is a GCP SM reference — i.e. a value this
// package wrote before mark8ly#621 retired the GCP Secret Manager backend.
// Never resolves any more (see ChainStore.Get); kept purely for
// classification so an unmigrated row fails with an explicit, named error
// instead of the generic "unrecognised reference" one.
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
//
// GCP Secret Manager itself was retired from this package in mark8ly#621
// (no code writes gsm:// references any more). This — and SecretResource,
// FormatReference, ParseReference below — stay only because
// cmd/carrier-secrets-backfill's tests use them to synthesise a legacy
// gsm:// reference to exercise the backfill's migration path; nothing in
// the production Store construction (Build, ChainStore) calls them.
func SecretName(prefix string, s Scope) string {
	tenant := encodeSegment(s.TenantID)
	domain := encodeSegment(strings.ToLower(s.Domain))
	provider := encodeSegment(strings.ToLower(s.Provider))
	field := encodeSegment(strings.ToLower(s.Field))
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
		encodeSegment(s.TenantID),
		encodeSegment(strings.ToLower(s.Domain)),
		encodeSegment(strings.ToLower(s.Provider)),
		encodeSegment(strings.ToLower(s.Field)),
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

// encodeSegment reduces a Scope segment to the character set both GCP
// Secret Manager names and OpenBao KV paths accept — [A-Za-z0-9_-] — while
// staying INJECTIVE: distinct inputs always produce distinct outputs. That
// matters because a Scope segment (e.g. a merchant-registered domain) is
// untrusted, and two distinct segments folding to the same encoded path
// would let one tenant's per-tenant carrier credential silently overwrite
// another's config in the same slot (see #606).
//
// Encoding, byte-wise (not rune-wise, so multi-byte UTF-8 becomes a run of
// escapes):
//   - '[A-Za-z0-9-]'  -> unchanged
//   - '_'             -> "__"
//   - anything else   -> '_' followed by its two-digit uppercase hex value
//
// decodeSegment is its exact inverse and exists to make injectivity
// testable (round-trip), not because any caller needs to decode a stored
// path — see below.
//
// Changing this function does NOT require a migration: every stored
// reference is self-describing (ParseBaoReference/ParseReference recover
// the path straight out of the reference string, never by recomputing it
// from a Scope), so old rows keep resolving at their old paths and only
// new writes land on paths produced by this version.
// upperHexDigits indexes the two-digit uppercase hex escape emitted by
// encodeSegment; decodeSegment parses it back with strconv.ParseUint.
const upperHexDigits = "0123456789ABCDEF"

func encodeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-':
			b.WriteByte(c)
		case c == '_':
			b.WriteString("__")
		default:
			// Hand-rolled hex rather than a formatting helper: the
			// log-shipping PII guard rejects the print-family calls in
			// service code, and suppressing that guard inside the encoder
			// for per-tenant secret paths would be the wrong signal. This
			// is also allocation-free.
			b.WriteByte('_')
			b.WriteByte(upperHexDigits[c>>4])
			b.WriteByte(upperHexDigits[c&0x0F])
		}
	}
	return b.String()
}

// decodeSegment is the inverse of encodeSegment. It returns an error on a
// malformed escape: a trailing lone '_', or a '_' followed by non-hex
// digits.
func decodeSegment(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '_' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("carriersecrets: malformed segment %q: trailing '_'", s)
		}
		if s[i+1] == '_' {
			b.WriteByte('_')
			i++
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("carriersecrets: malformed segment %q: incomplete escape at offset %d", s, i)
		}
		hexByte, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			return "", fmt.Errorf("carriersecrets: malformed segment %q: invalid hex escape at offset %d: %w", s, i, err)
		}
		b.WriteByte(byte(hexByte))
		i += 2
	}
	return b.String(), nil
}
