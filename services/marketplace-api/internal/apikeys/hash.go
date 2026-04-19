package apikeys

import (
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost — §18.4: 12 balances ~200ms verify against the 60s hot-key
// cache (so popular keys incur bcrypt once per minute, not per request).
// Test builds can lower via a build tag if needed.
const BcryptCost = 12

// dummyBcryptHash is a real bcrypt hash of a random string used for the
// timing-safe negative path: when the prefix lookup misses, we still run a
// bcrypt verify against this constant so the response time is comparable.
//
// Generated once: bcrypt.GenerateFromPassword([]byte("apikeys-timing-dummy"), 12).
var dummyBcryptHash = []byte("$2a$12$wKxPJF7lXFSNjm0xAHb1fOJkP6Y6T2J3R3XUpv4U.JmcW27bvQwoG")

// Hash bcrypts a plaintext key. Cost is fixed at BcryptCost; never accept a
// caller-supplied cost.
func Hash(plaintext string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Verify compares a stored hash against a plaintext key. Returns nil on
// match, bcrypt.ErrMismatchedHashAndPassword on mismatch.
func Verify(storedHash, plaintext string) error {
	return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(plaintext))
}

// VerifyDummy runs a bcrypt verify against the dummy hash. Always errors.
// Used by the auth middleware on prefix-miss to keep response timing parity
// with the prefix-hit-then-hash-miss path.
func VerifyDummy(plaintext string) error {
	return bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(plaintext))
}
