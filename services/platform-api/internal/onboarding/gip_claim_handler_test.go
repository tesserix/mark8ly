package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeClaimSetter struct {
	calls []struct{ uid, tenantID string }
	err   error
}

func (f *fakeClaimSetter) EnsureTenantClaim(_ context.Context, uid, tenantID string) error {
	f.calls = append(f.calls, struct{ uid, tenantID string }{uid, tenantID})
	return f.err
}

func TestGIPClaimOutboxHandler_WritesClaim(t *testing.T) {
	fake := &fakeClaimSetter{}
	h := NewGIPClaimOutboxHandler(fake)

	payload, _ := json.Marshal(gipClaimPayload{UserID: "uid-1", TenantID: "tenant-1"})
	if err := h(t.Context(), payload); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(fake.calls) != 1 {
		t.Fatalf("EnsureTenantClaim called %d times, want 1", len(fake.calls))
	}
	if fake.calls[0].uid != "uid-1" || fake.calls[0].tenantID != "tenant-1" {
		t.Errorf("called with %+v, want {uid-1 tenant-1}", fake.calls[0])
	}
}

// A failure must be returned so the drainer retries. Swallowing it would
// leave the owner permanently locked out of the mobile app — the exact
// failure mode this handler exists to prevent.
func TestGIPClaimOutboxHandler_PropagatesErrorForRetry(t *testing.T) {
	fake := &fakeClaimSetter{err: errors.New("identity toolkit unavailable")}
	h := NewGIPClaimOutboxHandler(fake)

	payload, _ := json.Marshal(gipClaimPayload{UserID: "uid-1", TenantID: "tenant-1"})
	if err := h(t.Context(), payload); err == nil {
		t.Fatal("handler swallowed the error; the drainer would mark it complete and never retry")
	}
}

func TestGIPClaimOutboxHandler_RejectsIncompletePayload(t *testing.T) {
	h := NewGIPClaimOutboxHandler(&fakeClaimSetter{})

	for _, tc := range []struct{ name, body string }{
		{"missing user_id", `{"tenant_id":"t1"}`},
		{"missing tenant_id", `{"user_id":"u1"}`},
		{"not json", `{`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := h(t.Context(), json.RawMessage(tc.body)); err == nil {
				t.Errorf("payload %q accepted, want error", tc.body)
			}
		})
	}
}
