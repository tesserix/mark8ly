package gip

import (
	"context"
	"errors"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────
// FakeVerifier sanity (used by every other test in the package)
// ─────────────────────────────────────────────────────────────────────────

func TestFakeVerifier_VerifyToken_HappyPath(t *testing.T) {
	f := NewFakeVerifier()
	f.Add("good-token", VerifiedToken{
		UID:      "user-1",
		Email:    "u@e.com",
		TenantID: "MP-Internal-test",
	})

	got, err := f.VerifyToken(context.Background(), "good-token", "MP-Internal-test")
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if got.UID != "user-1" {
		t.Errorf("UID = %q, want user-1", got.UID)
	}
}

func TestFakeVerifier_VerifyToken_RejectsUnknownToken(t *testing.T) {
	f := NewFakeVerifier()
	_, err := f.VerifyToken(context.Background(), "nope", "MP-Internal-test")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

// auth-bug #5 regression: a token issued for one tenant pool must NOT
// validate against a different pool, even if the signature is otherwise
// valid. The verifier explicitly compares the tenant claim.
func TestFakeVerifier_VerifyToken_RejectsTenantMismatch(t *testing.T) {
	f := NewFakeVerifier()
	f.Add("storefront-token", VerifiedToken{
		UID:      "user-1",
		TenantID: "MP-Customer-test",
	})

	// Caller expects the admin pool, but the token was issued from the
	// storefront pool. Must reject.
	_, err := f.VerifyToken(context.Background(), "storefront-token", "MP-Internal-test")
	if !errors.Is(err, ErrTenantMismatch) {
		t.Errorf("expected ErrTenantMismatch (auth-bug #5 regression), got %v", err)
	}
}

func TestFakeVerifier_FailNextN_SimulatesTransientFailure(t *testing.T) {
	f := NewFakeVerifier()
	f.Add("tok", VerifiedToken{UID: "u", TenantID: "T"})

	f.FailNextN(2)

	// First two calls fail.
	if _, err := f.VerifyToken(context.Background(), "tok", "T"); err == nil {
		t.Error("call 1 should fail")
	}
	if _, err := f.VerifyToken(context.Background(), "tok", "T"); err == nil {
		t.Error("call 2 should fail")
	}
	// Third call succeeds.
	if _, err := f.VerifyToken(context.Background(), "tok", "T"); err != nil {
		t.Errorf("call 3 should succeed, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Real verifier construction
// ─────────────────────────────────────────────────────────────────────────

func TestNew_RejectsEmptyProjectID(t *testing.T) {
	_, err := New(context.Background(), Config{})
	if err == nil {
		t.Error("New should reject empty ProjectID")
	}
}
