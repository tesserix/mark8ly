package authz

import (
	"context"
	"testing"
)

// TestFake_DeleteTuple_Idempotent pins the DeleteTuple contract on the
// in-memory fake: deleting an existing tuple removes it, and deleting
// an already-absent tuple is a no-op (nil error), matching the real
// OpenFGA-backed client's "already-deleted" tolerance.
func TestFake_DeleteTuple_Idempotent(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	if err := f.WriteOwnership(ctx, "u1", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteTuple(ctx, "u1", "owner", "t1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ok, _ := f.Check(ctx, "u1", "owner", "t1")
	if ok {
		t.Fatal("tuple still present after delete")
	}
	// Deleting again is a no-op, not an error.
	if err := f.DeleteTuple(ctx, "u1", "owner", "t1"); err != nil {
		t.Fatalf("second delete should be nil, got %v", err)
	}
}

// TestFake_DeleteStoreParent_Idempotent pins the DeleteStoreParent contract
// on the in-memory fake: deleting an existing store→tenant parent tuple
// removes it, and deleting an already-absent tuple is a no-op (nil error).
// This also pins the store-parent tuple's subject prefix (tenant: vs
// user:), since WriteStoreParent/DeleteStoreParent/HasStoreParent must all
// agree on the same storeID/tenantID pairing for this to pass.
func TestFake_DeleteStoreParent_Idempotent(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	if err := f.WriteStoreParent(ctx, "s1", "t1"); err != nil {
		t.Fatal(err)
	}
	if !f.HasStoreParent("s1", "t1") {
		t.Fatal("store parent tuple not present after write")
	}
	if err := f.DeleteStoreParent(ctx, "s1", "t1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if f.HasStoreParent("s1", "t1") {
		t.Fatal("store parent tuple still present after delete")
	}
	// Deleting again is a no-op, not an error.
	if err := f.DeleteStoreParent(ctx, "s1", "t1"); err != nil {
		t.Fatalf("second delete should be nil, got %v", err)
	}
}
