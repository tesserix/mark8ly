package carriersecrets

import (
	"context"
	"errors"
	"testing"
)

// TestFakeBao_SatisfiesSecretClientContract verifies the fake behaves
// like the real OpenBao backend on the contract ChainStore depends on:
// create-or-version, not-found mapping, and delete removes.
func TestFakeBao_SatisfiesSecretClientContract(t *testing.T) {
	ctx := context.Background()
	fb := NewFakeBao()

	// Test 1: CreateOrAddVersion creates and subsequent writes overwrite.
	secret := "test-secret"
	payload1 := []byte("value1")
	payload2 := []byte("value2")

	err := fb.CreateOrAddVersion(ctx, secret, payload1)
	if err != nil {
		t.Fatalf("CreateOrAddVersion (first write) failed: %v", err)
	}

	got, err := fb.AccessLatest(ctx, secret)
	if err != nil {
		t.Fatalf("AccessLatest after first write failed: %v", err)
	}
	if !bytesEqual(got, payload1) {
		t.Errorf("AccessLatest returned %q, want %q", got, payload1)
	}

	// Overwrite with second write.
	err = fb.CreateOrAddVersion(ctx, secret, payload2)
	if err != nil {
		t.Fatalf("CreateOrAddVersion (second write) failed: %v", err)
	}

	got, err = fb.AccessLatest(ctx, secret)
	if err != nil {
		t.Fatalf("AccessLatest after second write failed: %v", err)
	}
	if !bytesEqual(got, payload2) {
		t.Errorf("AccessLatest after overwrite returned %q, want %q", got, payload2)
	}

	// Test 2: AccessLatest returns ErrSecretNotFound for missing secret.
	notFound := "never-written"
	_, err = fb.AccessLatest(ctx, notFound)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("AccessLatest on missing secret returned %v, want ErrSecretNotFound", err)
	}

	// Test 3: DeleteSecret removes the value; subsequent read returns ErrSecretNotFound.
	err = fb.DeleteSecret(ctx, secret)
	if err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	_, err = fb.AccessLatest(ctx, secret)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("AccessLatest after delete returned %v, want ErrSecretNotFound", err)
	}

	// Test 4: DeleteSecret is idempotent — deleting absent secret succeeds.
	err = fb.DeleteSecret(ctx, "absent-key")
	if err != nil {
		t.Errorf("DeleteSecret on absent key returned %v, want nil", err)
	}
}

// bytesEqual is a helper to compare byte slices for testing.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
