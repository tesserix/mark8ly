// Package adminhandoff verifies HMAC-signed handoff codes minted by the
// admin app to bridge a session across TLDs.
//
// Why this exists: the admin session cookie (m8_session) is AES-GCM
// encrypted JSON scoped to .mark8ly.com. When a merchant's tenant has a
// custom admin domain (admin.<merchant-tld>), the cookie can't ride
// cross-TLD with the navigation, so the destination loops back to the
// canonical login. The handoff is a short-lived (30s) signed envelope
// carrying the user's identity claims; the receiving handler verifies
// it, runs an FGA membership check, and mints a fresh m8_session
// scoped to the custom-admin host.
//
// Wire format (mirrors @repo/ui/auth/admin-handoff-code on the TS
// side): `<base64url-payload>.<hex-sha256>`. The HMAC key is the same
// SESSION_ENCRYPT_KEY used to AES-encrypt the session cookie itself —
// only services that already mint sessions can mint handoffs, which
// constrains the trust boundary.
package adminhandoff

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CodeKind is the discriminator embedded in the JWT-style envelope.
// Pinning this prevents a storefront-trampoline exchange code (same
// HMAC key, different shape) from ever being redeemed at the admin
// handoff endpoint.
const CodeKind = "admin_handoff_v1"

// Claims is the payload carried inside an admin-handoff code.
type Claims struct {
	Kind       string `json:"kind"`
	UID        string `json:"uid"`
	Email      string `json:"email"`
	TenantID   string `json:"tenant_id"`
	StoreID    string `json:"store_id,omitempty"`
	TargetHost string `json:"target_host"`
	Exp        int64  `json:"exp"` // Unix epoch seconds
}

// Errors returned by Verify. Callers map these to HTTP status codes.
var (
	ErrMissingKey       = errors.New("adminhandoff: missing HMAC key")
	ErrMalformed        = errors.New("adminhandoff: malformed code")
	ErrInvalidSignature = errors.New("adminhandoff: invalid signature")
	ErrInvalidPayload   = errors.New("adminhandoff: payload is not valid JSON")
	ErrExpired          = errors.New("adminhandoff: code expired")
	ErrWrongKind        = errors.New("adminhandoff: wrong code kind")
	ErrIncompleteClaims = errors.New("adminhandoff: incomplete claims")
)

// Verify decodes a code, checks HMAC + expiry + kind + non-empty
// identity claims. Returns the parsed claims on success.
//
// Caller is still responsible for: (1) verifying the request host
// matches claims.TargetHost (defense in depth — a stolen code shouldn't
// be redeemable on a different host) and (2) running an FGA membership
// check on (UID, TenantID).
func Verify(code, key string) (*Claims, error) {
	if key == "" {
		return nil, ErrMissingKey
	}

	dot := strings.LastIndex(code, ".")
	if dot < 0 {
		return nil, ErrMalformed
	}
	payload := code[:dot]
	sig := code[dot+1:]

	expected := signRaw([]byte(payload), key)
	provided, err := hex.DecodeString(sig)
	if err != nil {
		return nil, ErrInvalidSignature
	}
	if !hmac.Equal(provided, expected) {
		return nil, ErrInvalidSignature
	}

	rawJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrInvalidPayload
	}

	var claims Claims
	if err := json.Unmarshal(rawJSON, &claims); err != nil {
		return nil, ErrInvalidPayload
	}

	if claims.Kind != CodeKind {
		return nil, fmt.Errorf("%w: got %q want %q", ErrWrongKind, claims.Kind, CodeKind)
	}
	if time.Now().Unix() >= claims.Exp {
		return nil, ErrExpired
	}
	if claims.UID == "" || claims.TenantID == "" || claims.TargetHost == "" {
		return nil, ErrIncompleteClaims
	}

	return &claims, nil
}

func signRaw(payload []byte, key string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return mac.Sum(nil)
}
