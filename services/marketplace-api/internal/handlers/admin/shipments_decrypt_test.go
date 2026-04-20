package admin

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// TestShipmentsHandler_decryptCarrierCreds_WithEncryptor pins the
// round-trip between the settings handler (which encrypts) and the
// shipments handler (which must decrypt). When this regression slipped
// in before, the carrier saw the raw ciphertext as its bearer token
// and every request failed auth — so the Delhivery dashboard showed
// zero orders despite shipments being "created" in our own DB. Future
// refactors that skip the decryption step will trip this test.
func TestShipmentsHandler_decryptCarrierCreds_WithEncryptor(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	apiCipher, err := enc.Encrypt("real-delhivery-token")
	if err != nil {
		t.Fatalf("encrypt api: %v", err)
	}
	secretCipher, err := enc.Encrypt("real-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	h := (&ShipmentsHandler{}).WithEncryptor(enc)

	cfg := &shipping.CarrierConfig{APIKey: apiCipher, SecretKey: secretCipher}
	apiPlain, secretPlain, err := h.decryptCarrierCreds(cfg)
	if err != nil {
		t.Fatalf("decryptCarrierCreds: %v", err)
	}
	if apiPlain != "real-delhivery-token" {
		t.Errorf("api plaintext = %q, want real-delhivery-token", apiPlain)
	}
	if secretPlain != "real-secret" {
		t.Errorf("secret plaintext = %q, want real-secret", secretPlain)
	}
}

// TestShipmentsHandler_decryptCarrierCreds_NoEncryptor guards the
// backwards-compat path: a deployment that never enabled envelope
// encryption must still be able to read its existing plaintext keys.
// Without this, flipping on the fix would break dev setups running
// with the previous schema.
func TestShipmentsHandler_decryptCarrierCreds_NoEncryptor(t *testing.T) {
	h := &ShipmentsHandler{}
	cfg := &shipping.CarrierConfig{APIKey: "raw-key", SecretKey: "raw-secret"}
	apiPlain, secretPlain, err := h.decryptCarrierCreds(cfg)
	if err != nil {
		t.Fatalf("decryptCarrierCreds without encryptor: %v", err)
	}
	if apiPlain != "raw-key" || secretPlain != "raw-secret" {
		t.Errorf("pass-through = (%q, %q), want (raw-key, raw-secret)", apiPlain, secretPlain)
	}
}
