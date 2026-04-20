package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// TestShippingSettings_WithSecretStore_PutAndMask is the handler-level
// pin for the write/read contract: an admin who saves a Delhivery
// config with a carriersecrets.FakeStore wired must see the DB row
// carry a "gsm://" reference AND the masked response preserve the
// plaintext tail. If this test fails, the hybrid-store wiring broke
// somewhere between putCredential and maskKeyField.
func TestShippingSettings_WithSecretStore_PutAndMask(t *testing.T) {
	store := carriersecrets.NewFakeStore("tesseracthub-480811", "mark8ly-test", crypto.NewNoopEncryptor())
	h := (&ShippingSettingsHandler{}).WithSecretStore(store)
	ctx := context.Background()

	ref, err := h.putCredential(ctx, "4a47610c-3f0c-4ef7-a64c-892480c4635e", "delhivery", "api_key", "real-token-wxyz")
	if err != nil {
		t.Fatalf("putCredential: %v", err)
	}
	if !strings.HasPrefix(ref, "gsm://") {
		t.Fatalf("reference not a gsm:// value: %s", ref)
	}
	wantRef := "gsm://projects/tesseracthub-480811/secrets/mark8ly-test-4a47610c-3f0c-4ef7-a64c-892480c4635e-shipping-delhivery-api_key"
	if ref != wantRef {
		t.Fatalf("unexpected ref: got %q want %q", ref, wantRef)
	}
	if got := h.maskKeyField(ctx, ref); got != "****wxyz" {
		t.Errorf("mask = %q, want ****wxyz", got)
	}
}

// TestShippingSettings_WithoutSecretStore_FallbackToEncryptor asserts
// the backwards-compat path: a handler constructed only with an
// Encryptor (pre-rollout deployment) must keep encrypting +
// decrypting into the api_key_encrypted column without change.
func TestShippingSettings_WithoutSecretStore_FallbackToEncryptor(t *testing.T) {
	h := &ShippingSettingsHandler{encryptor: crypto.NewNoopEncryptor()}
	ctx := context.Background()
	ref, err := h.putCredential(ctx, "t", "delhivery", "api_key", "legacy-abcd")
	if err != nil {
		t.Fatalf("putCredential: %v", err)
	}
	if strings.HasPrefix(ref, "gsm://") {
		t.Fatalf("Encryptor-only handler unexpectedly produced a gsm:// reference: %s", ref)
	}
	if !strings.HasPrefix(ref, "noop:") {
		t.Fatalf("expected noop: prefix, got %s", ref)
	}
	if got := h.maskKeyField(ctx, ref); got != "****abcd" {
		t.Errorf("mask = %q, want ****abcd", got)
	}
}

// TestPaymentSettings_WithSecretStore_ScopeIsPaymentDomain guards
// against a copy-paste that would file Razorpay keys under the
// "shipping" domain in GCP SM. An IAM binding scoped to
// `resource.name.startsWith("…-shipping-…")` would silently deny
// access without this.
func TestPaymentSettings_WithSecretStore_ScopeIsPaymentDomain(t *testing.T) {
	store := carriersecrets.NewFakeStore("proj", "pfx", crypto.NewNoopEncryptor())
	h := (&PaymentSettingsHandler{}).WithSecretStore(store)
	ctx := context.Background()
	ref, err := h.putCredential(ctx, "tenant-1", "razorpay", "api_key", "rzp_test_xxxx")
	if err != nil {
		t.Fatalf("putCredential: %v", err)
	}
	if !strings.Contains(ref, "-payment-razorpay-api_key") {
		t.Fatalf("reference doesn't carry payment scope: %s", ref)
	}
}

// TestTaxSettings_WithSecretStore_ScopeIsTaxDomain mirrors the
// payment-domain guard for TaxJar credentials.
func TestTaxSettings_WithSecretStore_ScopeIsTaxDomain(t *testing.T) {
	store := carriersecrets.NewFakeStore("proj", "pfx", crypto.NewNoopEncryptor())
	h := (&TaxSettingsHandler{}).WithSecretStore(store)
	ctx := context.Background()
	ref, err := h.putCredential(ctx, "tenant-1", "taxjar", "api_key", "tj_live_zzzz")
	if err != nil {
		t.Fatalf("putCredential: %v", err)
	}
	if !strings.Contains(ref, "-tax-taxjar-api_key") {
		t.Fatalf("reference doesn't carry tax scope: %s", ref)
	}
}

// TestMaskStoredKey_Behaviour exercises the cross-cutting resolver
// mask the way a handler's response DTO uses it. Empty -> empty;
// nil store -> ****; gsm reference -> plaintext tail.
func TestMaskStoredKey_Behaviour(t *testing.T) {
	ctx := context.Background()
	// nil store
	if got := maskStoredKey(ctx, nil, "anything"); got != "****" {
		t.Errorf("nil store mask = %q, want ****", got)
	}
	// empty ref
	store := carriersecrets.NewFakeStore("p", "pfx", crypto.NewNoopEncryptor())
	if got := maskStoredKey(ctx, store, ""); got != "" {
		t.Errorf("empty ref mask = %q, want empty", got)
	}
	// round-trip
	ref, err := store.Put(ctx, carriersecrets.Scope{TenantID: "t", Domain: "shipping", Provider: "x", Field: "api_key"}, "super-secret-tail1234")
	if err != nil {
		t.Fatal(err)
	}
	if got := maskStoredKey(ctx, store, ref); got != "****1234" {
		t.Errorf("round-trip mask = %q, want ****1234", got)
	}
}
