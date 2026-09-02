package journal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// UnsubscribeTokenLength is the fixed length of a hex-encoded unsubscribe
// token: 32 random bytes at 2 hex characters each. Matches the
// journal_subscribers.unsubscribe_token CHAR(64) column added by
// migration 000125.
const UnsubscribeTokenLength = 64

// unsubscribeTokenBytes is the amount of entropy fed into GenerateUnsubscribeToken,
// before hex encoding. 32 bytes (256 bits) is comfortably beyond brute-force
// range for a bearer credential with no other rate limiting than the
// unsubscribe endpoint's own IP-based limiter.
const unsubscribeTokenBytes = 32

// GenerateUnsubscribeToken returns a cryptographically random, hex-encoded
// bearer token that authorises deleting exactly one journal_subscribers
// row. It is generated with crypto/rand — never derived from the
// subscriber's email or any other value the server already holds, since a
// derived token would be computable by anyone who later learns (or
// guesses) the address it protects.
func GenerateUnsubscribeToken() (string, error) {
	buf := make([]byte, unsubscribeTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("journal: generate unsubscribe token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
