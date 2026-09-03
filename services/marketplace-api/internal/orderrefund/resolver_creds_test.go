package orderrefund

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// Regression: GatewayFor passed payment_gateway_configs.api_key_encrypted /
// secret_key_encrypted straight into payment.NewGateway. Those columns hold
// a REFERENCE, not a credential — "gsm://projects/.../secrets/..." (now
// "bao://kv/..." — GCP Secret Manager was retired in mark8ly#621) once the
// row has been rewrapped. Razorpay was therefore handed the literal
// reference string as its API key and answered
//
//	401 {"code":"BAD_REQUEST_ERROR","description":"Authentication failed"}
//
// so every refund 500'd. Observed in prod 2026-07-17 on store my-god
// (order 3936309a-587f-4060-943a-b4a4983305c8).

func TestResolveCred_ResolvesBaoReference(t *testing.T) {
	store := carriersecrets.NewFakeStore(crypto.NewNoopEncryptor())
	r := (&Resolver{}).WithSecretStore(store)
	ctx := context.Background()

	const plaintext = "rzp_test_realkey123"
	ref, err := store.Put(ctx, carriersecrets.Scope{
		TenantID: "0d7d8563-f155-4520-8238-45e646e4d8fa",
		Domain:   "payment",
		Provider: "razorpay",
		Field:    "api_key",
	}, plaintext)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.HasPrefix(ref, "bao://") {
		t.Fatalf("precondition: want bao:// ref, got %q", ref)
	}

	got, err := r.resolveCred(ctx, ref)
	if err != nil {
		t.Fatalf("resolveCred(bao ref): %v", err)
	}
	if got != plaintext {
		t.Errorf("credential = %q, want %q", got, plaintext)
	}
	// The whole point: a raw reference must never reach the gateway.
	if strings.HasPrefix(got, "bao://") {
		t.Error("resolveCred returned the raw reference — gateway would 401")
	}
}

func TestResolveCred_InlineEncryptorFallback(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	r := (&Resolver{}).WithEncryptor(enc)
	ctx := context.Background()

	const plaintext = "legacy_inline_key"
	cipher, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := r.resolveCred(ctx, cipher)
	if err != nil {
		t.Fatalf("resolveCred(inline): %v", err)
	}
	if got != plaintext {
		t.Errorf("credential = %q, want %q", got, plaintext)
	}
}

// An unwired Resolver must fail loudly rather than pass the reference
// through — silently forwarding it is what produced the opaque 401.
func TestResolveCred_UnwiredFailsLoudlyInsteadOfLeakingRef(t *testing.T) {
	r := &Resolver{}
	got, err := r.resolveCred(context.Background(), "gsm://projects/p/secrets/s")
	if err == nil {
		t.Fatalf("want an error, got credential %q", got)
	}
	if got != "" {
		t.Errorf("must not return the raw reference, got %q", got)
	}
}

func TestResolveCred_EmptyRefIsNotAnError(t *testing.T) {
	r := (&Resolver{}).WithSecretStore(
		carriersecrets.NewFakeStore(crypto.NewNoopEncryptor()),
	)
	got, err := r.resolveCred(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveCred(\"\"): %v", err)
	}
	if got != "" {
		t.Errorf("credential = %q, want empty", got)
	}
}
