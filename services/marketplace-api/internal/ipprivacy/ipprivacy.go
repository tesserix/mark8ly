// Package ipprivacy provides HMAC-SHA256 hashing of client IP addresses for
// privacy-preserving audit logging. The HMAC key lives in Secret Manager and
// is rotated quarterly; the same plaintext IP under different keys produces
// different hashes (so historical observations can't be cross-referenced
// after rotation), but within one key window, repeat observations of the
// same IP are linkable.
//
// Used by P7 (tax attestation) inline, and now by P14 (api-key last-used).
// Centralised here so future callers don't reinvent the hash construction.
package ipprivacy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
)

// Hasher is a key-bound HMAC-SHA256 IP hasher. Construct once at boot with
// the secret loaded from Secret Manager; reuse for every request.
type Hasher struct {
	key []byte
}

// New returns a Hasher bound to the given key. An empty key returns a
// no-op hasher whose Hash method always returns "" — useful for local dev.
func New(key []byte) *Hasher {
	return &Hasher{key: key}
}

// Hash returns the hex-encoded HMAC-SHA256 of the IP. Returns "" when the
// hasher has no key or the input is unparseable. Strips a trailing :port if
// present so the same client gets the same hash across requests.
func (h *Hasher) Hash(ip string) string {
	if h == nil || len(h.key) == 0 {
		return ""
	}
	cleaned := normalize(ip)
	if cleaned == "" {
		return ""
	}
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write([]byte(cleaned))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalize(ip string) string {
	raw := strings.TrimSpace(ip)
	if raw == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	if net.ParseIP(raw) == nil {
		return ""
	}
	return raw
}
