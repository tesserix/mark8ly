// Package crypto — keyutil.go: utilities for decoding encryption keys.
package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// DecodeKey decodes a 32-byte encryption key from hex or base64.
// Hex format: 64 characters (32 bytes when decoded).
// Base64 format: 44 characters with padding (32 bytes when decoded).
func DecodeKey(s string) ([]byte, error) {
	// Try hex first (64 chars = 32 bytes).
	if len(s) == 64 {
		b, err := hex.DecodeString(s)
		if err == nil && len(b) == 32 {
			return b, nil
		}
	}
	// Fall back to base64 (44 chars = 32 bytes with padding).
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("key is neither valid hex nor base64: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("decoded key is %d bytes, want 32", len(b))
	}
	return b, nil
}
