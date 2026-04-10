// Package crypto provides envelope encryption for sensitive data at rest
// (API keys, tokens). Production uses AES-256-GCM with a DEK wrapped by
// GCP KMS. Local dev uses a noop encoder (base64) so `make dev` works
// without GCP credentials.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// Encryptor encrypts and decrypts sensitive strings for storage.
type Encryptor interface {
	Encrypt(plaintext string) (ciphertext string, err error)
	Decrypt(ciphertext string) (plaintext string, err error)
}

// --- Noop (dev) ---

// NoopEncryptor base64-encodes values without real encryption.
// Suitable for local development only.
type NoopEncryptor struct{}

func NewNoopEncryptor() *NoopEncryptor { return &NoopEncryptor{} }

func (NoopEncryptor) Encrypt(plaintext string) (string, error) {
	return "noop:" + base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (NoopEncryptor) Decrypt(ciphertext string) (string, error) {
	const prefix = "noop:"
	if len(ciphertext) < len(prefix) || ciphertext[:len(prefix)] != prefix {
		// Not a noop-encrypted value — return as-is for migration compatibility.
		return ciphertext, nil
	}
	b, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", fmt.Errorf("noop decrypt: %w", err)
	}
	return string(b), nil
}

// --- AES-256-GCM (production) ---

// AESEncryptor uses AES-256-GCM for envelope encryption.
// The key must be exactly 32 bytes (256 bits).
type AESEncryptor struct {
	gcm cipher.AEAD
}

// NewAESEncryptor creates an AES-256-GCM encryptor from a 32-byte key.
func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return &AESEncryptor{gcm: gcm}, nil
}

func (e *AESEncryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "aes:" + base64.StdEncoding.EncodeToString(sealed), nil
}

func (e *AESEncryptor) Decrypt(ciphertext string) (string, error) {
	const prefix = "aes:"
	if len(ciphertext) < len(prefix) || ciphertext[:len(prefix)] != prefix {
		return "", fmt.Errorf("not an AES-encrypted value")
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := e.gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
