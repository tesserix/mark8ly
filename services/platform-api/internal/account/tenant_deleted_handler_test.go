package account

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTenantPurger struct {
	calls []struct {
		tenantID string
		storeIDs []string
	}
	err error
}

func (f *fakeTenantPurger) PurgeTenant(_ context.Context, tenantID string, storeIDs []string) error {
	f.calls = append(f.calls, struct {
		tenantID string
		storeIDs []string
	}{tenantID, storeIDs})
	return f.err
}

func TestTenantDeletedHandler_PurgesTenant(t *testing.T) {
	fake := &fakeTenantPurger{}
	h := NewTenantDeletedHandler(fake)

	payload, _ := json.Marshal(tenantDeletedPayload{TenantID: "t1", StoreIDs: []string{"s1"}})
	if err := h(t.Context(), payload); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(fake.calls) != 1 {
		t.Fatalf("PurgeTenant called %d times, want 1", len(fake.calls))
	}
	if fake.calls[0].tenantID != "t1" {
		t.Errorf("tenant_id = %q, want %q", fake.calls[0].tenantID, "t1")
	}
	if len(fake.calls[0].storeIDs) != 1 || fake.calls[0].storeIDs[0] != "s1" {
		t.Errorf("store_ids = %v, want [s1]", fake.calls[0].storeIDs)
	}
}

// A failure must be returned unchanged so the drainer retries. Swallowing
// it would mark the event complete and silently orphan the tenant's
// marketplace data — the exact failure mode this handler exists to
// prevent.
func TestTenantDeletedHandler_PropagatesErrorForRetry(t *testing.T) {
	wantErr := errors.New("marketplace-api unreachable")
	fake := &fakeTenantPurger{err: wantErr}
	h := NewTenantDeletedHandler(fake)

	payload, _ := json.Marshal(tenantDeletedPayload{TenantID: "t1", StoreIDs: []string{"s1"}})
	err := h(t.Context(), payload)
	if err == nil {
		t.Fatal("handler swallowed the error; the drainer would mark it complete and never retry")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestTenantDeletedHandler_MalformedPayloadReturnsError(t *testing.T) {
	fake := &fakeTenantPurger{}
	h := NewTenantDeletedHandler(fake)

	if err := h(t.Context(), json.RawMessage(`{`)); err == nil {
		t.Error("malformed payload accepted, want error")
	}
	if len(fake.calls) != 0 {
		t.Errorf("PurgeTenant called on malformed payload, want 0 calls")
	}
}

func TestTenantDeletedHandler_MissingTenantIDReturnsError(t *testing.T) {
	fake := &fakeTenantPurger{}
	h := NewTenantDeletedHandler(fake)

	payload, _ := json.Marshal(tenantDeletedPayload{StoreIDs: []string{"s1"}})
	if err := h(t.Context(), payload); err == nil {
		t.Error("missing tenant_id accepted, want error")
	}
	if len(fake.calls) != 0 {
		t.Errorf("PurgeTenant called with missing tenant_id, want 0 calls")
	}
}
