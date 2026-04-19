package arbitrage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HashResult is what Hasher returns. KeyVersion is stored alongside the audit
// row (in metadata, not ip_hash itself) so correlation code knows which key
// to use when comparing two rows across rotation windows.
type HashResult struct {
	Hex        string // HMAC-SHA256 hex, always 64 chars, or "" if no IP
	KeyVersion string // Secret Manager version resource name
}

// Hasher is the one-and-only path for IP → ip_hash. Every write of
// subscription_arbitrage_audit.ip_hash MUST go through this type.
// Task 15 enforces this via grep: no other file may call crypto/hmac for IP work.
type Hasher struct {
	loader *KeyLoader
}

// NewHasher constructs a Hasher backed by the given KeyLoader.
func NewHasher(loader *KeyLoader) *Hasher {
	return &Hasher{loader: loader}
}

// Hash computes HMAC-SHA256(key=latest_secret_version, data=rawIP) and returns
// the hex digest + the key's Secret Manager version name.
//
// This is the CORRECT construction: hmac.New(sha256.New, key) is keyed-hash
// by design and is length-extension-safe. Do NOT replace with
// sha256.Sum256(append(salt, ip...)) — that is NOT a MAC and is vulnerable
// to length-extension attacks per spec §18.8 security review.
func (h *Hasher) Hash(ctx context.Context, rawIP string) (HashResult, error) {
	if rawIP == "" {
		return HashResult{}, nil
	}
	key, version, err := h.loader.Latest(ctx)
	if err != nil {
		return HashResult{}, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(rawIP))
	digest := mac.Sum(nil)
	return HashResult{
		Hex:        hex.EncodeToString(digest),
		KeyVersion: version,
	}, nil
}
