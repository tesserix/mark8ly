// Package platformadmin serves mark8ly's /admin/* surface to the Tesserix
// platform console (#274). It is deliberately separate from
// internal/handlers/admin: different auth chain, different response
// envelope, different audience. The two share the domain packages beneath
// them and nothing at the HTTP layer.
//
// # Signature scheme
//
// This package is the reference implementation of mark8ly's request-signing
// scheme for /admin/* calls. It is specified nowhere else — the console team
// implements against this code and the golden vectors in testdata/vectors.json
// (published on #275). A few properties matter enough to call out explicitly:
//
//   - The signed Path is c.Request.URL.Path — already
//     percent-decoded by net/http — never RawPath, never EscapedPath, and
//     never the raw wire form of the request target. A caller that signs
//     the wire-form path (e.g. "/tenants/t%20one") while the server signs
//     the decoded form (e.g. "/tenants/t one", with a real space) produces
//     a different canonical string and every percent-encoded path 401s. See
//     testdata vector "repeated-query-and-encoded-path", whose "path" field
//     is the decoded form actually signed and whose "request_target" field
//     is the raw wire form shown only for illustration.
//   - Query values are escaped with application/x-www-form-urlencoded
//     semantics (Go's url.QueryEscape): a space becomes "+", and a literal
//     "+" becomes "%2B". This diverges from encodeURIComponent-style
//     escaping (JavaScript, Python's urllib.parse.quote), which encodes a
//     space as "%20" and leaves "+" as data. An implementation built on
//     encodeURIComponent must convert "%20" to "+" (and must not
//     double-escape an existing "+") to match this canonicaliser, or every
//     query value containing a space silently 401s — see testdata vector
//     "query-value-with-space". Only the *output* escaping diverges:
//     url.ParseQuery decodes both "%20" and "+" to a space on input, so the
//     raw query string the caller happened to build with is irrelevant —
//     what matters is how CanonicalQuery re-escapes it on the way out.
//   - Method, Path, Timestamp, Nonce, Operator and Capability must not
//     contain '\n' or '\r'. CanonicalString enforces this and returns an
//     error otherwise. The canonical string joins fields with "\n" and has
//     no length prefixes, so without this guard two different inputs could
//     produce identical bytes (e.g. Operator="a", Capability="b\nc" collides
//     with Operator="a\nb", Capability="c"). This is not exploitable today —
//     RawQuery is percent-escaped by CanonicalQuery, Body is folded into a
//     fixed-width hash, and net/http itself rejects '\n' in header values —
//     but the invariant is enforced here rather than left accidental,
//     because Path is populated from a decoded URL and a literal '%0A' in a
//     request path decodes to a real newline in Request.URL.Path.
//   - Sign always emits lowercase hex. Verify accepts either case: some
//     client stacks (.NET's BitConverter.ToString, several Java HMAC
//     helpers) emit uppercase hex by default, and a naive string-equality
//     check against that would fail with an unexplained 401.
//   - Sign and Verify reject an empty secret. An unconfigured secret
//     reaching this layer is a misconfiguration, not something that should
//     silently produce a valid-looking HMAC.
package platformadmin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Header names carried by every signed platform call.
const (
	HeaderOperator   = "X-Platform-Operator"
	HeaderCapability = "X-Platform-Capability"
	HeaderTimestamp  = "X-Platform-Timestamp"
	HeaderNonce      = "X-Platform-Nonce"
	HeaderSignature  = "X-Platform-Signature"
)

// SignatureInput is everything covered by the HMAC. Operator and capability
// are signed so neither can be substituted after signing — they are the
// attribution the whole surface exists to record.
//
// Path must be the decoded net/http Request.URL.Path, not RawPath or
// EscapedPath — see the package doc.
type SignatureInput struct {
	Method     string
	Path       string
	RawQuery   string
	Body       []byte
	Timestamp  string
	Nonce      string
	Operator   string
	Capability string
}

// CanonicalQuery renders a query string deterministically: keys sorted, then
// values within a repeated key sorted, each percent-encoded, joined by "&".
// Both sides must agree byte-for-byte, so nothing here may depend on map
// iteration order.
func CanonicalQuery(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", fmt.Errorf("platformadmin: parse query: %w", err)
	}

	keys := make([]string, 0, len(values))
	total := 0
	for k, vs := range values {
		keys = append(keys, k)
		total += len(vs)
	}
	sort.Strings(keys)

	parts := make([]string, 0, total)
	for _, k := range keys {
		vs := append([]string(nil), values[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&"), nil
}

// noLineBreakFields are the components joined by "\n" in CanonicalString
// that are not otherwise protected from ambiguity (RawQuery is
// percent-escaped by CanonicalQuery; Body is folded into a fixed-width
// hash). Order is fixed so a multi-field violation always reports the same
// field first.
func checkNoLineBreaks(in SignatureInput) error {
	fields := []struct {
		name, value string
	}{
		{"method", in.Method},
		{"path", in.Path},
		{"timestamp", in.Timestamp},
		{"nonce", in.Nonce},
		{"operator", in.Operator},
		{"capability", in.Capability},
	}
	for _, f := range fields {
		if strings.ContainsAny(f.value, "\n\r") {
			return fmt.Errorf("platformadmin: %s must not contain a newline or carriage return", f.name)
		}
	}
	return nil
}

// CanonicalString builds the string the HMAC covers. The body is included as
// a hash rather than inline so a captured signature cannot be lifted onto a
// different payload. An absent body hashes as the empty string.
func CanonicalString(in SignatureInput) (string, error) {
	if err := checkNoLineBreaks(in); err != nil {
		return "", err
	}

	query, err := CanonicalQuery(in.RawQuery)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(in.Body)

	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(in.Method)),
		in.Path,
		query,
		hex.EncodeToString(sum[:]),
		in.Timestamp,
		in.Nonce,
		in.Operator,
		in.Capability,
	}, "\n"), nil
}

// Sign returns the hex HMAC-SHA256 of the canonical string, always in
// lowercase. It rejects an empty secret: an unconfigured secret reaching
// this layer is a misconfiguration that should be loud rather than
// producing a valid-looking HMAC.
func Sign(secret string, in SignatureInput) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("platformadmin: secret must not be empty")
	}

	canonical, err := CanonicalString(in)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify compares a presented signature against the expected one in constant
// time. It accepts hex in either case — some client stacks emit uppercase
// hex by default — by decoding both sides to bytes before comparing, so an
// uppercase-hex signature from such a client does not present as an
// unexplained 401. A malformed (non-hex) presented signature is treated as a
// failed verification rather than a caller error, since it is
// indistinguishable from a client with a mismatched signature. A malformed
// query or an empty secret still yields an error, so the caller can
// distinguish "bad request"/"misconfigured" from "bad signature" in logs
// while still returning one opaque status to the client.
func Verify(secret, got string, in SignatureInput) (bool, error) {
	want, err := Sign(secret, in)
	if err != nil {
		return false, err
	}

	gotRaw, err := hex.DecodeString(got)
	if err != nil {
		return false, nil
	}
	wantRaw, err := hex.DecodeString(want)
	if err != nil {
		return false, err
	}
	return hmac.Equal(gotRaw, wantRaw), nil
}
