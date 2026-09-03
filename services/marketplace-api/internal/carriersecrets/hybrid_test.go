package carriersecrets

import (
	"context"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// TestHybridStore_Put_WritesGSMReference pins the write path: a Put
// against a HybridStore must persist the plaintext in GCP SM (via
// the FakeClient) and return a "gsm://" reference that round-trips
// back through Get.
func TestHybridStore_Put_WritesGSMReference(t *testing.T) {
	fake := NewFakeClient()
	s := NewHybridStore(HybridConfig{
		Client:    fake,
		Encryptor: crypto.NewNoopEncryptor(),
		ProjectID: "tesseracthub-480811",
		Prefix:    "mark8ly-test",
	})
	ctx := context.Background()
	scope := Scope{
		TenantID: "4a47610c-3f0c-4ef7-a64c-892480c4635e",
		Domain:   "shipping",
		Provider: "delhivery",
		Field:    "api_key",
	}
	ref, err := s.Put(ctx, scope, "real-delhivery-token")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// "api_key" contains a literal '_', which encodeSegment escapes to "__"
	// to stay injective (see #606).
	wantRef := "gsm://projects/tesseracthub-480811/secrets/mark8ly-test-4a47610c-3f0c-4ef7-a64c-892480c4635e-shipping-delhivery-api__key"
	if ref != wantRef {
		t.Fatalf("ref = %q, want %q", ref, wantRef)
	}
	if !fake.Has("projects/tesseracthub-480811/secrets/mark8ly-test-4a47610c-3f0c-4ef7-a64c-892480c4635e-shipping-delhivery-api__key") {
		t.Fatal("FakeClient doesn't have the secret after Put")
	}
	got, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "real-delhivery-token" {
		t.Fatalf("Get returned %q, want real-delhivery-token", got)
	}
}

// TestHybridStore_Get_ResolvesInlineRef_BackwardsCompat pins the
// migration-compat contract: a row whose api_key_encrypted still
// holds a noop:/aes: value must decode successfully so switching
// SHIPPING_SECRET_STORE=gcpsm never takes an existing row offline.
func TestHybridStore_Get_ResolvesInlineRef_BackwardsCompat(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	legacy, err := enc.Encrypt("legacy-token")
	if err != nil {
		t.Fatal(err)
	}
	s := NewHybridStore(HybridConfig{
		Client:    NewFakeClient(),
		Encryptor: enc,
		ProjectID: "p",
		Prefix:    "mark8ly-dev",
	})
	got, err := s.Get(context.Background(), legacy)
	if err != nil {
		t.Fatalf("Get legacy: %v", err)
	}
	if got != "legacy-token" {
		t.Fatalf("got %q, want legacy-token", got)
	}
}

// TestHybridStore_Get_EmptyRef returns empty plaintext so the admin
// mask helper can draw a blank badge rather than erroring out.
func TestHybridStore_Get_EmptyRef(t *testing.T) {
	s := NewHybridStore(HybridConfig{
		Client:    NewFakeClient(),
		Encryptor: crypto.NewNoopEncryptor(),
		ProjectID: "p",
		Prefix:    "pfx",
	})
	got, err := s.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("Get empty: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// TestHybridStore_MaybeRewrap_MigratesInlineToGSM is the lazy
// auto-migration pin. A handler that sees a legacy noop: row on
// read must get back a new gsm:// reference + changed=true so it
// can UPDATE the DB column. Subsequent MaybeRewrap calls on the
// new reference are no-ops.
func TestHybridStore_MaybeRewrap_MigratesInlineToGSM(t *testing.T) {
	enc := crypto.NewNoopEncryptor()
	legacy, _ := enc.Encrypt("legacy-delhivery")
	s := NewHybridStore(HybridConfig{
		Client:    NewFakeClient(),
		Encryptor: enc,
		ProjectID: "tesseracthub-480811",
		Prefix:    "mark8ly-prod",
	})
	ctx := context.Background()
	scope := Scope{TenantID: "tenant-1", Domain: "shipping", Provider: "delhivery", Field: "api_key"}

	newRef, changed := s.MaybeRewrap(ctx, legacy, scope, "legacy-delhivery")
	if !changed {
		t.Fatal("expected changed=true for legacy ref")
	}
	if !IsGSMRef(newRef) {
		t.Fatalf("new ref not a gsm ref: %s", newRef)
	}

	// Subsequent MaybeRewrap against the new reference: no-op.
	_, changed2 := s.MaybeRewrap(ctx, newRef, scope, "legacy-delhivery")
	if changed2 {
		t.Fatal("expected changed=false for already-migrated ref")
	}

	// Get against the new reference returns the same plaintext.
	got, err := s.Get(ctx, newRef)
	if err != nil {
		t.Fatal(err)
	}
	if got != "legacy-delhivery" {
		t.Fatalf("plaintext after migration: got %q, want legacy-delhivery", got)
	}
}

// TestHybridStore_MaybeRewrap_NoopForEmpty guards the empty-column
// path: an empty api_key_encrypted never triggers an SM write, so we
// don't create zero-value secrets for rows that were never saved
// (e.g. a tenant who removed their Razorpay config before this code
// shipped).
func TestHybridStore_MaybeRewrap_NoopForEmpty(t *testing.T) {
	s := NewHybridStore(HybridConfig{
		Client:    NewFakeClient(),
		Encryptor: crypto.NewNoopEncryptor(),
		ProjectID: "p",
		Prefix:    "pfx",
	})
	_, changed := s.MaybeRewrap(context.Background(), "", Scope{TenantID: "t", Domain: "shipping", Provider: "delhivery", Field: "api_key"}, "")
	if changed {
		t.Fatal("expected changed=false on empty ref")
	}
}

// TestHybridStore_Destroy_RemovesSecret pins the Delete path —
// admin delete endpoints must clean up the detached resource so IAM
// audits don't show dangling secrets for removed carrier configs.
func TestHybridStore_Destroy_RemovesSecret(t *testing.T) {
	fc := NewFakeClient()
	s := NewHybridStore(HybridConfig{
		Client:    fc,
		Encryptor: crypto.NewNoopEncryptor(),
		ProjectID: "p",
		Prefix:    "pfx",
	})
	ctx := context.Background()
	scope := Scope{TenantID: "t", Domain: "shipping", Provider: "delhivery", Field: "api_key"}
	ref, err := s.Put(ctx, scope, "v")
	if err != nil {
		t.Fatal(err)
	}
	res, _ := ParseReference(ref)
	if !fc.Has(res) {
		t.Fatal("secret not written")
	}
	if err := s.Destroy(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if fc.Has(res) {
		t.Fatal("secret still present after Destroy")
	}
}

// TestHybridStore_Destroy_InlineIsNoop guards the mixed-deployment
// path: a tenant whose row still carries a noop:/aes: value
// shouldn't blow up a delete — the ciphertext lives only in the DB
// row the caller is about to drop.
func TestHybridStore_Destroy_InlineIsNoop(t *testing.T) {
	s := NewHybridStore(HybridConfig{
		Client:    NewFakeClient(),
		Encryptor: crypto.NewNoopEncryptor(),
		ProjectID: "p",
		Prefix:    "pfx",
	})
	if err := s.Destroy(context.Background(), "noop:YWJj"); err != nil {
		t.Fatalf("Destroy on inline ref errored: %v", err)
	}
}
