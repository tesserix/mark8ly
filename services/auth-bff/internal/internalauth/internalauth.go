// Package internalauth holds the one shared-secret scheme auth-bff uses to
// authenticate server-to-server callers.
//
// The scheme predates this package: internal/audit and internal/notify send
// the secret as an X-Internal-Auth header on outbound calls, and
// internal/session's /internal/users handler validates the same header on
// the way in. This package is that inbound check, lifted out so every route
// that needs it compares the same way instead of growing its own copy.
//
// The secret itself is never logged, echoed, or included in any error.
package internalauth

import (
	"crypto/sha256"
	"crypto/subtle"
)

// Header is the request header carrying the shared internal secret.
const Header = "X-Internal-Auth"

// Equal reports whether the presented secret matches the expected one, in
// constant time. An empty value on either side never matches, so a caller
// that sends no header is rejected exactly like one that sends a wrong
// value.
//
// Both sides are hashed first so the comparison is over fixed-length inputs
// and the length of the configured secret does not leak through timing.
func Equal(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}
