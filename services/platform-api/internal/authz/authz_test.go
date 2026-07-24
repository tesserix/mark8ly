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
