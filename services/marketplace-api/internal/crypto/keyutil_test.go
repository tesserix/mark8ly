// Package crypto — keyutil_test.go: tests for key decoding utilities.
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// TestDecodeKey_Hex verifies hex-encoded key decoding.
func TestDecodeKey_Hex(t *testing.T) {
	// Generate a random 32-byte key and hex-encode it.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	hexKey := hex.EncodeToString(key)
	if len(hexKey) != 64 {
		t.Fatalf("hex key length = %d, want 64", len(hexKey))
	}

	decoded, err := DecodeKey(hexKey)
	if err != nil {
		t.Fatalf("DecodeKey(hex) error = %v", err)
	}

	if len(decoded) != 32 {
		t.Errorf("decoded length = %d, want 32", len(decoded))
	}

	for i := range key {
		if decoded[i] != key[i] {
			t.Errorf("decoded[%d] = %d, want %d", i, decoded[i], key[i])
		}
	}
}

// TestDecodeKey_Base64 verifies base64-encoded key decoding.
func TestDecodeKey_Base64(t *testing.T) {
	// Generate a random 32-byte key and base64-encode it.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	b64Key := base64.StdEncoding.EncodeToString(key)
	if len(b64Key) != 44 {
		t.Fatalf("base64 key length = %d, want 44", len(b64Key))
	}

	decoded, err := DecodeKey(b64Key)
	if err != nil {
		t.Fatalf("DecodeKey(base64) error = %v", err)
	}

	if len(decoded) != 32 {
		t.Errorf("decoded length = %d, want 32", len(decoded))
	}

	for i := range key {
		if decoded[i] != key[i] {
			t.Errorf("decoded[%d] = %d, want %d", i, decoded[i], key[i])
		}
	}
}

// TestDecodeKey_HexPriority verifies hex is tried before base64 for 64-char
// strings that happen to also be valid base64.
func TestDecodeKey_HexPriority(t *testing.T) {
	// A valid hex string that is also valid base64.
	// 64 hex chars (32 bytes) that decode to the same bytes via hex.
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	decoded, err := DecodeKey(hexKey)
	if err != nil {
		t.Fatalf("DecodeKey() error = %v", err)
	}

	// Verify it was decoded as hex.
	expected, _ := hex.DecodeString(hexKey)
	for i := range expected {
		if decoded[i] != expected[i] {
			t.Errorf("decoded[%d] = %d, want %d (hex decode)", i, decoded[i], expected[i])
		}
	}
}

// TestDecodeKey_InvalidInputs verifies error handling for invalid keys.
func TestDecodeKey_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"too_short_hex", "0123456789abcdef"},
		{"invalid_hex_chars", "ghijklmnopqrstuvwxyzGHIJKLMNOPQRSTUVWXYZ0123456789abcdef0123"},
		{"wrong_length_base64", base64.StdEncoding.EncodeToString(make([]byte, 16))}, // 16 bytes
		{"invalid_base64", "!!!not-valid-base64!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeKey(tt.key)
			if err == nil {
				t.Errorf("DecodeKey(%q) expected error, got nil", tt.key)
			}
		})
	}
}

// TestDecodeKey_KnownVectors verifies decoding with known test vectors.
func TestDecodeKey_KnownVectors(t *testing.T) {
	// 32 bytes of zeros in hex.
	zeroHex := "0000000000000000000000000000000000000000000000000000000000000000"
	decoded, err := DecodeKey(zeroHex)
	if err != nil {
		t.Fatalf("DecodeKey(zero hex) error = %v", err)
	}
	for i, b := range decoded {
		if b != 0 {
			t.Errorf("decoded[%d] = %d, want 0", i, b)
		}
	}

	// 32 bytes of 0xFF in hex.
	ffHex := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	decoded, err = DecodeKey(ffHex)
	if err != nil {
		t.Fatalf("DecodeKey(ff hex) error = %v", err)
	}
	for i, b := range decoded {
		if b != 0xFF {
			t.Errorf("decoded[%d] = %d, want 255", i, b)
		}
	}

	// 32 bytes of zeros in base64.
	zeroB64 := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	decoded, err = DecodeKey(zeroB64)
	if err != nil {
		t.Fatalf("DecodeKey(zero base64) error = %v", err)
	}
	for i, b := range decoded {
		if b != 0 {
			t.Errorf("decoded[%d] = %d, want 0", i, b)
		}
	}
}

// TestDecodeKey_EndToEnd verifies a decoded key works with AESEncryptor.
func TestDecodeKey_EndToEnd(t *testing.T) {
	// Generate a random key, encode as hex, decode, use for encryption.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	hexKey := hex.EncodeToString(key)
	decoded, err := DecodeKey(hexKey)
	if err != nil {
		t.Fatalf("DecodeKey() error = %v", err)
	}

	enc, err := NewAESEncryptor(decoded)
	if err != nil {
		t.Fatalf("NewAESEncryptor() error = %v", err)
	}

	plaintext := "sk_test_secret_api_key_12345"
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}
