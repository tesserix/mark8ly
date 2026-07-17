package storefront

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// Regression: verify-payment resolved the Razorpay signing secret through
// the Encryptor directly, which only understands the legacy inline "aes:"
// envelope. Once a payment_gateway_configs row had been rewrapped its
// secret_key_encrypted column holds a "gsm://" reference, so every
// verification failed with `not an AES-encrypted value` and returned
// 503 {"error":"gateway not configured"} — the customer paid at Razorpay
// and the order stayed pending. Observed in prod 2026-07-17 on
// store my-god (order M-MYG-260717-00001).
//
// The fix routes verify-payment through decryptAPIKey, which resolves via
// the carriersecrets.Store when wired. These tests pin that contract for
// both storage formats.

func TestDecryptAPIKey_ResolvesGSMReference(t *testing.T) {
	store := carriersecrets.NewFakeStore("tesseracthub-480811", "mark8ly-test", crypto.NewNoopEncryptor())
	h := (&WebhookHandler{}).WithSecretStore(store)
	ctx := context.Background()

	const plaintext = "rzp-signing-secret-abc123"
	ref, err := store.Put(ctx, carriersecrets.Scope{
		TenantID: "0d7d8563-f155-4520-8238-45e646e4d8fa",
		Domain:   "payment",
		Provider: "razorpay",
		Field:    "secret_key",
	}, plaintext)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.HasPrefix(ref, "gsm://") {
		t.Fatalf("precondition: want a gsm:// reference, got %q", ref)
	}

	// This is the exact call verify-payment makes. Before the fix the
	// equivalent Encryptor-only path returned "not an AES-encrypted value".
	got, err := h.decryptAPIKey(ctx, ref)
	if err != nil {
		t.Fatalf("decryptAPIKey(gsm ref) failed — verify-payment would 503: %v", err)
	}
	if got != plaintext {
		t.Errorf("plaintext = %q, want %q", got, plaintext)
	}
}

// Deployments still running inline-mode (no Store wired) must keep working
// off the Encryptor column, so the fix can't regress them.
func TestDecryptAPIKey_FallsBackToEncryptorForInlineCiphertext(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	h := &WebhookHandler{encryptor: enc}
	ctx := context.Background()

	const plaintext = "legacy-inline-secret"
	cipher, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := h.decryptAPIKey(ctx, cipher)
	if err != nil {
		t.Fatalf("decryptAPIKey(inline) failed: %v", err)
	}
	if got != plaintext {
		t.Errorf("plaintext = %q, want %q", got, plaintext)
	}
}

// An unset webhook_secret_encrypted column is what pushed the prod row onto
// the secret_key_encrypted fallback in the first place; an empty reference
// must resolve to empty rather than erroring.
func TestDecryptAPIKey_EmptyReference(t *testing.T) {
	h := (&WebhookHandler{}).WithSecretStore(
		carriersecrets.NewFakeStore("p", "mark8ly-test", crypto.NewNoopEncryptor()),
	)
	got, err := h.decryptAPIKey(context.Background(), "")
	if err != nil {
		t.Fatalf("decryptAPIKey(\"\") = %v", err)
	}
	if got != "" {
		t.Errorf("plaintext = %q, want empty", got)
	}
}
