// Package platformadmin serves mark8ly's /admin/* surface to the Tesserix
// platform console (#274). It is deliberately separate from
// internal/handlers/admin: different auth chain, different response
// envelope, different audience. The two share the domain packages beneath
// them and nothing at the HTTP layer.
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
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(values))
	for _, k := range keys {
		vs := append([]string(nil), values[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&"), nil
}

// CanonicalString builds the string the HMAC covers. The body is included as
// a hash rather than inline so a captured signature cannot be lifted onto a
// different payload. An absent body hashes as the empty string.
func CanonicalString(in SignatureInput) (string, error) {
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

// Sign returns the hex HMAC-SHA256 of the canonical string.
func Sign(secret string, in SignatureInput) (string, error) {
	canonical, err := CanonicalString(in)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify compares a presented signature against the expected one in constant
// time. A malformed query yields an error rather than a false negative, so
// the caller can distinguish "bad request" from "bad signature" in logs while
// still returning one opaque status to the client.
func Verify(secret, got string, in SignatureInput) (bool, error) {
	want, err := Sign(secret, in)
	if err != nil {
		return false, err
	}
	return hmac.Equal([]byte(got), []byte(want)), nil
}
