// Package customerportal implements the GDPR customer-subject data portal
// (§15.4 — Art.17 / Art.20 rights). It exposes:
//
//   - GET  /storefront/stores/:storeSlug/my-orders/:email/:order_token
//     — no auth; HMAC token IS the auth; 90-day post-closure window check.
//   - POST /storefront/stores/:storeSlug/customer-erasure
//     — append-only insert into customer_erasure_requests.
package customerportal

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// tokenSep is the separator used inside the HMAC message.
const tokenSep = "|"

// SecretLength is the byte length of the per-store HMAC secret (32 bytes → 64
// hex chars stored in stores.storefront_customer_portal_secret).
const SecretLength = 32

// GenerateSecret generates a new random 64-char hex secret for a store.
// Called lazily when a store's portal is first accessed and the column is empty.
func GenerateSecret() (string, error) {
	b := make([]byte, SecretLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("customerportal: generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// BuildToken constructs an HMAC-SHA256 token for the given email and orderID,
// signed with the per-store secret. Token format (base64url-encoded):
//
//	hmac_sha256(secret, email | "|" | orderID)
//
// The token has no embedded expiry; expiry is enforced server-side via the
// 90-day post-closure window check in the handler (§15.4).
func BuildToken(secret, email, orderID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join([]string{email, orderID}, tokenSep)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyToken checks that token matches BuildToken(secret, email, orderID).
// Uses constant-time comparison to prevent timing attacks.
func VerifyToken(secret, email, orderID, token string) bool {
	expected := BuildToken(secret, email, orderID)
	// Decode both to raw bytes for constant-time comparison.
	expectedBytes, err1 := base64.RawURLEncoding.DecodeString(expected)
	actualBytes, err2 := base64.RawURLEncoding.DecodeString(token)
	if err1 != nil || err2 != nil {
		return false
	}
	return hmac.Equal(expectedBytes, actualBytes)
}
