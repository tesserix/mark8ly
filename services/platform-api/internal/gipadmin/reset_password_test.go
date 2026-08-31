package gipadmin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

// The oobCode is minted inside a GIP tenant (SendPasswordResetOobCode
// sends tenantId), so it only resolves when it is redeemed in that same
// tenant. Omitting tenantId here makes Identity Toolkit look the code up
// at project level, where it does not exist — GIP then answers
// INVALID_OOB_CODE and the merchant is told their brand-new link is
// "invalid or has expired". Config documents TenantID as "sent as
// tenantId on every call"; this pins that for the redeem call.
func TestResetPasswordSendsTenantID(t *testing.T) {
	var got map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	if err := c.ResetPassword(context.Background(), "oob-123", "correct horse battery staple"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	if got["tenantId"] != "MP-Internal-e986p" {
		t.Errorf("tenantId = %v, want %q", got["tenantId"], "MP-Internal-e986p")
	}
	// Guard the rest of the payload so adding tenantId cannot silently
	// drop what GIP actually needs to perform the reset.
	if got["oobCode"] != "oob-123" {
		t.Errorf("oobCode = %v, want %q", got["oobCode"], "oob-123")
	}
	if got["newPassword"] != "correct horse battery staple" {
		t.Errorf("newPassword = %v", got["newPassword"])
	}
}

// An expired code must still map to ErrInvalidOobCode once tenantId is
// present, so the handler keeps returning 410 rather than a 500.
func TestResetPasswordExpiredCodeMapsToSentinel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"EXPIRED_OOB_CODE","code":400}}`))
	})

	err := c.ResetPassword(context.Background(), "oob-123", "correct horse battery staple")
	if !errors.Is(err, ErrInvalidOobCode) {
		t.Fatalf("err = %v, want ErrInvalidOobCode", err)
	}
}
