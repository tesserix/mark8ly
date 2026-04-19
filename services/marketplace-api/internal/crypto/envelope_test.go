// Package crypto — envelope_test.go: unit tests for envelope encryption.
package crypto

import (
	"crypto/rand"
	"strings"
	"testing"
)

// TestNoopEncryptor_RoundTrip verifies noop encrypt/decrypt cycle.
func TestNoopEncryptor_RoundTrip(t *testing.T) {
	enc := NewNoopEncryptor()

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty", ""},
		{"simple", "sk_test_1234567890"},
		{"stripe_key", "sk_live_51ABC123DEFxyzSecretKeyData"},
		{"razorpay_key", "rzp_test_ABCDEFghij1234567890"},
		{"with_special", "key!@#$%^&*()_+{}|:<>?[]\\;',./"},
		{"unicode", "密钥_κλειδί_مفتاح"},
		{"long", strings.Repeat("a", 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if !strings.HasPrefix(ciphertext, "noop:") {
				t.Errorf("Encrypt() ciphertext missing 'noop:' prefix, got %q", ciphertext)
			}

			decrypted, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

// TestNoopEncryptor_LegacyPlaintext verifies noop passes through plaintext
// values that were stored before encryption was enabled (migration compat).
func TestNoopEncryptor_LegacyPlaintext(t *testing.T) {
	enc := NewNoopEncryptor()

	// A raw plaintext value that would have been stored pre-encryption.
	legacy := "sk_test_1234567890"

	decrypted, err := enc.Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt(legacy) error = %v", err)
	}

	// Should return the value as-is for migration compatibility.
	if decrypted != legacy {
		t.Errorf("Decrypt(legacy) = %q, want %q", decrypted, legacy)
	}
}

// TestAESEncryptor_RoundTrip verifies AES-256-GCM encrypt/decrypt cycle.
func TestAESEncryptor_RoundTrip(t *testing.T) {
	// Generate a random 32-byte key.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	enc, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESEncryptor() error = %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty", ""},
		{"simple", "sk_test_1234567890"},
		{"stripe_key", "sk_live_51ABC123DEFxyzSecretKeyData"},
		{"razorpay_key", "rzp_test_ABCDEFghij1234567890"},
		{"taxjar_key", "tj_test_abc123xyz789"},
		{"with_special", "key!@#$%^&*()_+{}|:<>?[]\\;',./"},
		{"unicode", "密钥_κλειδί_مفتاح"},
		{"long", strings.Repeat("a", 4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if !strings.HasPrefix(ciphertext, "aes:") {
				t.Errorf("Encrypt() ciphertext missing 'aes:' prefix, got %q", ciphertext[:min(len(ciphertext), 20)])
			}

			decrypted, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

// TestAESEncryptor_UniqueNonce verifies each encryption produces different
// ciphertext due to random nonce.
func TestAESEncryptor_UniqueNonce(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	enc, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESEncryptor() error = %v", err)
	}

	plaintext := "sk_test_repeated"
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		ciphertext, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt() iteration %d error = %v", i, err)
		}
		if seen[ciphertext] {
			t.Fatalf("Encrypt() produced duplicate ciphertext at iteration %d", i)
		}
		seen[ciphertext] = true
	}
}

// TestAESEncryptor_InvalidKeySize verifies key size validation.
func TestAESEncryptor_InvalidKeySize(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{"too_short_16", 16},
		{"too_short_24", 24},
		{"too_long_48", 48},
		{"too_long_64", 64},
		{"empty", 0},
		{"one_byte", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			_, err := NewAESEncryptor(key)
			if err == nil {
				t.Errorf("NewAESEncryptor(%d bytes) expected error, got nil", tt.keySize)
			}
		})
	}
}

// TestAESEncryptor_DecryptInvalidPrefix verifies error on non-AES values.
func TestAESEncryptor_DecryptInvalidPrefix(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	enc, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESEncryptor() error = %v", err)
	}

	tests := []struct {
		name       string
		ciphertext string
	}{
		{"plaintext", "sk_test_1234567890"},
		{"noop_prefix", "noop:c2tfdGVzdF8xMjM0NTY3ODkw"},
		{"wrong_prefix", "foo:SGVsbG8gV29ybGQ="},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := enc.Decrypt(tt.ciphertext)
			if err == nil {
				t.Errorf("Decrypt(%q) expected error, got nil", tt.ciphertext)
			}
		})
	}
}

// TestAESEncryptor_DecryptCorrupted verifies error on corrupted ciphertext.
func TestAESEncryptor_DecryptCorrupted(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	enc, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESEncryptor() error = %v", err)
	}

	// Encrypt a value, then corrupt the ciphertext.
	ciphertext, err := enc.Encrypt("sk_test_1234567890")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Corrupt by replacing a base64 char with one that's guaranteed to
	// differ. Picking a fixed substitute (e.g. always "X") flakes when
	// the original char happens to equal the substitute (~1/64 chance
	// since base64 has 64 chars), turning the "corruption" into a no-op.
	pos := len("aes:") + 5
	orig := ciphertext[pos]
	sub := byte('X')
	if orig == sub {
		sub = 'Y'
	}
	corrupted := ciphertext[:pos] + string(sub) + ciphertext[pos+1:]

	_, err = enc.Decrypt(corrupted)
	if err == nil {
		t.Errorf("Decrypt(corrupted) expected error, got nil")
	}
}

// TestAESEncryptor_DecryptWrongKey verifies error when decrypting with
// a different key.
func TestAESEncryptor_DecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	if _, err := rand.Read(key1); err != nil {
		t.Fatalf("generate key1: %v", err)
	}
	if _, err := rand.Read(key2); err != nil {
		t.Fatalf("generate key2: %v", err)
	}

	enc1, err := NewAESEncryptor(key1)
	if err != nil {
		t.Fatalf("NewAESEncryptor(key1) error = %v", err)
	}
	enc2, err := NewAESEncryptor(key2)
	if err != nil {
		t.Fatalf("NewAESEncryptor(key2) error = %v", err)
	}

	ciphertext, err := enc1.Encrypt("sk_test_secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	_, err = enc2.Decrypt(ciphertext)
	if err == nil {
		t.Errorf("Decrypt(wrong key) expected error, got nil")
	}
}

// TestEncryptorInterface verifies both implementations satisfy the interface.
func TestEncryptorInterface(t *testing.T) {
	var _ Encryptor = (*NoopEncryptor)(nil)
	var _ Encryptor = (*AESEncryptor)(nil)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
