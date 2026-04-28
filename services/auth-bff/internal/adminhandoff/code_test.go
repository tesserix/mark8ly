package adminhandoff_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mark8ly/auth-bff/internal/adminhandoff"
)

const testKey = "thirtytwo-bytes-for-testing-only"

// mintForTest produces a handoff code via the same signing scheme the
// TS @repo/ui/auth/admin-handoff-code helper uses, so the Go verifier
// is exercised against the wire format that the admin app produces.
func mintForTest(t *testing.T, claims adminhandoff.Claims, key string) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func validClaims() adminhandoff.Claims {
	return adminhandoff.Claims{
		Kind:       adminhandoff.CodeKind,
		UID:        "uid-123",
		Email:      "merchant@primasyss.com",
		TenantID:   "tenant-india-store",
		StoreID:    "store-india-store",
		TargetHost: "admin.primasyss.com",
		Exp:        time.Now().Add(30 * time.Second).Unix(),
	}
}

func TestVerify_HappyPath(t *testing.T) {
	in := validClaims()
	code := mintForTest(t, in, testKey)

	out, err := adminhandoff.Verify(code, testKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.UID != in.UID || out.TenantID != in.TenantID || out.TargetHost != in.TargetHost {
		t.Fatalf("Verify: claims mismatch: got %+v", out)
	}
}

func TestVerify_RejectsWrongKey(t *testing.T) {
	code := mintForTest(t, validClaims(), testKey)
	if _, err := adminhandoff.Verify(code, "another-32-byte-key-for-testing!"); !errors.Is(err, adminhandoff.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerify_RejectsExpired(t *testing.T) {
	c := validClaims()
	c.Exp = time.Now().Add(-time.Second).Unix()
	code := mintForTest(t, c, testKey)

	if _, err := adminhandoff.Verify(code, testKey); !errors.Is(err, adminhandoff.ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestVerify_RejectsWrongKind(t *testing.T) {
	c := validClaims()
	c.Kind = "storefront_exchange"
	code := mintForTest(t, c, testKey)

	if _, err := adminhandoff.Verify(code, testKey); !errors.Is(err, adminhandoff.ErrWrongKind) {
		t.Fatalf("expected ErrWrongKind, got %v", err)
	}
}

func TestVerify_RejectsMalformed(t *testing.T) {
	if _, err := adminhandoff.Verify("not-a-code", testKey); !errors.Is(err, adminhandoff.ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestVerify_RejectsIncompleteClaims(t *testing.T) {
	c := validClaims()
	c.UID = ""
	code := mintForTest(t, c, testKey)
	if _, err := adminhandoff.Verify(code, testKey); !errors.Is(err, adminhandoff.ErrIncompleteClaims) {
		t.Fatalf("expected ErrIncompleteClaims, got %v", err)
	}
}

func TestVerify_RejectsEmptyKey(t *testing.T) {
	code := mintForTest(t, validClaims(), testKey)
	if _, err := adminhandoff.Verify(code, ""); !errors.Is(err, adminhandoff.ErrMissingKey) {
		t.Fatalf("expected ErrMissingKey, got %v", err)
	}
}

func TestVerify_RejectsTamperedPayload(t *testing.T) {
	code := mintForTest(t, validClaims(), testKey)
	// Flip a single byte of the payload (before the dot). Signature
	// won't match.
	tampered := []byte(code)
	tampered[0] ^= 0x01
	if _, err := adminhandoff.Verify(string(tampered), testKey); err == nil {
		t.Fatalf("expected error for tampered payload, got nil")
	}
}
