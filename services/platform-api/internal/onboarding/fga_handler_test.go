package onboarding

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark8ly/platform-api/internal/authz"
)

// TestFGAOutboxHandler_WritesBothTuples is the regression test for
// auth-bug #2 (FGA tuple race) — the membership tuple must always be
// written, and we want to know it as soon as the handler runs.
func TestFGAOutboxHandler_WritesBothTuples(t *testing.T) {
	fake := authz.NewFake()
	handler := NewFGAOutboxHandler(fake)

	payload, _ := json.Marshal(fgaWritePayload{
		UserID:   "user-abc",
		TenantID: "tenant-xyz",
	})
	if err := handler(context.Background(), payload); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !fake.HasOwnership("user-abc", "tenant-xyz") {
		t.Error("ownership tuple was not written")
	}
	if !fake.HasMembership("user-abc", "tenant-xyz") {
		t.Error("membership tuple was not written")
	}
}

// TestFGAOutboxHandler_RetriesOnFailure is the regression test for
// auth-bug #3 (no retry on FGA failure). The handler returns an error,
// the (real) drainer would mark the row for retry, then on the next tick
// the handler runs again and succeeds.
//
// The handler aborts on the FIRST write failure, so we fail-once to simulate
// the first invocation, and the second invocation finds the fake clean.
func TestFGAOutboxHandler_RetriesOnFailure(t *testing.T) {
	fake := authz.NewFake()
	fake.FailNextWrites(1) // ownership write fails the first invocation; second is clean
	handler := NewFGAOutboxHandler(fake)

	payload, _ := json.Marshal(fgaWritePayload{
		UserID:   "user-abc",
		TenantID: "tenant-xyz",
	})

	// First call: fails (the simulated FGA outage).
	if err := handler(context.Background(), payload); err == nil {
		t.Error("handler should have failed on first call")
	}
	if fake.HasOwnership("user-abc", "tenant-xyz") {
		t.Error("ownership should not be written on failed call")
	}

	// Second call (drainer retry): succeeds.
	if err := handler(context.Background(), payload); err != nil {
		t.Errorf("handler should succeed on retry, got %v", err)
	}
	if !fake.HasOwnership("user-abc", "tenant-xyz") {
		t.Error("ownership should be written after retry")
	}
	if !fake.HasMembership("user-abc", "tenant-xyz") {
		t.Error("membership should be written after retry")
	}
}

// TestFGAOutboxHandler_RejectsMalformedPayload guards against accidentally
// dispatching events with the wrong shape.
func TestFGAOutboxHandler_RejectsMalformedPayload(t *testing.T) {
	handler := NewFGAOutboxHandler(authz.NewFake())

	cases := []struct {
		name    string
		payload []byte
	}{
		{"invalid_json", []byte("not json")},
		{"missing_user_id", []byte(`{"tenant_id":"x"}`)},
		{"missing_tenant_id", []byte(`{"user_id":"x"}`)},
		{"empty_object", []byte(`{}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := handler(context.Background(), tc.payload); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
