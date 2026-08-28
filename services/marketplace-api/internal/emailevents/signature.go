// Package emailevents receives provider delivery events for outbound mail
// (#348, piece B) and applies them to the email_sends rows piece A writes.
package emailevents

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrBadSignature means the payload was not signed by the configured
	// secret, or was altered after signing.
	ErrBadSignature = errors.New("emailevents: signature mismatch")
	// ErrStaleSignature means the delivery is outside the freshness window,
	// in either direction.
	ErrStaleSignature = errors.New("emailevents: signature timestamp outside tolerance")
	// ErrMalformedHeader means a required signature header was absent or
	// unparseable.
	ErrMalformedHeader = errors.New("emailevents: malformed signature headers")
	// ErrNotConfigured means no signing secret is set.
	//
	// Returned rather than passing: a verifier that accepts when it has
	// nothing to check against is worse than no verifier, because it looks
	// like protection.
	ErrNotConfigured = errors.New("emailevents: no webhook signing secret configured")
)

// SecretPrefix is the marker Resend's signing secrets carry. The bytes after
// it are base64 — the HMAC key is those DECODED bytes, not the printable
// string, which is the detail an implementation is most likely to get wrong
// and still appear to work against its own self-signed fixtures.
const SecretPrefix = "whsec_"

// maxAge bounds replay. A captured delivery stays signature-valid forever
// without it; five minutes matches the Stripe verifier in
// internal/billing/stripe/signature.go.
const maxAge = 5 * time.Minute

// Verify checks a provider delivery against the configured signing secret.
//
// The signed payload is "{id}.{timestamp}.{raw body}", HMAC-SHA256 with the
// decoded key, base64-encoded. The signature header carries a space-separated
// list of versioned signatures ("v1,<sig> v1,<sig>") so a provider can rotate
// keys without a flag day; any v1 entry matching is enough.
//
// rawBody MUST be the bytes as received. Re-marshalling parsed JSON changes
// key order and whitespace, and the signature is over the exact bytes.
func Verify(rawBody []byte, id, timestamp, signature, secret string, now time.Time) error {
	if strings.TrimSpace(secret) == "" {
		return ErrNotConfigured
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(timestamp) == "" ||
		strings.TrimSpace(signature) == "" {
		return ErrMalformedHeader
	}

	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return ErrMalformedHeader
	}
	if diff := now.Sub(time.Unix(ts, 0)); diff > maxAge || diff < -maxAge {
		// Both directions: a far-future timestamp is as suspect as a stale
		// one, and only bounding the past leaves a signature valid forever
		// by dating it forward.
		return ErrStaleSignature
	}

	key, err := decodeSecret(secret)
	if err != nil {
		return err
	}

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(ts, 10) + "."))
	mac.Write(rawBody)
	expected := mac.Sum(nil)

	for _, part := range strings.Fields(signature) {
		version, encoded, ok := strings.Cut(part, ",")
		if !ok || version != "v1" {
			// Unknown scheme versions are skipped, not trusted. A future v2
			// must be implemented deliberately rather than accepted by
			// default.
			continue
		}
		got, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		// hmac.Equal, never ==: constant time, so a mismatch does not leak
		// how much of the signature was correct.
		if hmac.Equal(expected, got) {
			return nil
		}
	}
	return ErrBadSignature
}

// decodeSecret strips the prefix and base64-decodes the key.
func decodeSecret(secret string) ([]byte, error) {
	raw := strings.TrimSpace(secret)
	raw = strings.TrimPrefix(raw, SecretPrefix)
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrMalformedHeader
	}
	if len(key) == 0 {
		return nil, ErrNotConfigured
	}
	return key, nil
}
