package admin

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// TestMaskEncryptedKey_ReturnsPlaintextTail pins the display contract:
// the admin UI must show the last four chars of the *plaintext* key,
// not the ciphertext. Previously the shipping settings response masked
// the ciphertext column directly, which produced values like "****0Q=="
// (the base64 tail of AES output) — unrecognizable to the merchant.
func TestMaskEncryptedKey_ReturnsPlaintextTail(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	ciphertext, err := enc.Encrypt("abcdef1234567890wxyz")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got := maskEncryptedKey(enc, ciphertext)
	if got != "****wxyz" {
		t.Errorf("masked = %q, want ****wxyz", got)
	}
	// Guard: the ciphertext tail ("cHdYWVo=") must never leak through.
	if strings.Contains(got, "=") {
		t.Errorf("masked leaked ciphertext characters: %q", got)
	}
}

// TestMaskEncryptedKey_EmptyCiphertext returns "" so the UI can hide
// the masked badge entirely (secret keys are optional for most carriers).
func TestMaskEncryptedKey_EmptyCiphertext(t *testing.T) {
	if got := maskEncryptedKey(crypto.NewNoopEncryptor(), ""); got != "" {
		t.Errorf("empty ciphertext = %q, want empty string", got)
	}
}

// TestMaskEncryptedKey_DecryptFailure returns a neutral "****" rather
// than leaking the ciphertext. This matters when the encryption mode
// rotates (noop → aes) and an old row can no longer be decoded — we
// must never expose the raw column as if it were the real key.
func TestMaskEncryptedKey_DecryptFailure(t *testing.T) {
	aesKey := make([]byte, 32)
	for i := range aesKey {
		aesKey[i] = byte(i)
	}
	aes, err := crypto.NewAESEncryptor(aesKey)
	if err != nil {
		t.Fatalf("new aes: %v", err)
	}
	// Feed a noop-encrypted value into the aes decryptor — mismatched
	// prefix should make Decrypt error.
	stale, _ := crypto.NewNoopEncryptor().Encrypt("legacy-token-1234")
	got := maskEncryptedKey(aes, stale)
	if got != "****" {
		t.Errorf("decrypt-failure mask = %q, want ****", got)
	}
}

// TestMaskEncryptedKey_NilEncryptor is defensive: if the handler is
// constructed without an encryptor (tests, misconfiguration) we still
// refuse to reveal any part of the stored ciphertext.
func TestMaskEncryptedKey_NilEncryptor(t *testing.T) {
	if got := maskEncryptedKey(nil, "anything"); got != "****" {
		t.Errorf("nil encryptor mask = %q, want ****", got)
	}
}
