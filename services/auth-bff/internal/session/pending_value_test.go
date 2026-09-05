package session

import (
	"errors"
	"testing"
	"time"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	// 32 bytes of obviously-fake, low-entropy key material: a random-looking
	// literal here reads as a credential to secret scanners.
	m, err := NewManager(Config{
		EncryptKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CookieName: "m8_session",
		Domain:     "mark8ly.com",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// The mobile step-up resumes from a sealed value rather than a cookie. It
// must round-trip every field the resume needs — including the Zitadel
// session handle, without which the challenge cannot mint a token.
func TestSealOpenPending_RoundTrips(t *testing.T) {
	m := testManager(t)
	// Values are long and distinctive on purpose: the leak check below is a
	// substring scan, and a short value like "t1" appears in base64 output
	// by coincidence, which fails the test for the wrong reason.
	in := Pending{
		UID:                 "user-389396765696066342",
		Email:               "merchant@example.test",
		TenantID:            "tenant-e638b731-6a49-48ce",
		ZitadelSessionID:    "zitadel-session-identifier",
		ZitadelSessionToken: "zitadel-session-token-value",
	}

	sealed, err := m.SealPending(in)
	if err != nil {
		t.Fatalf("SealPending: %v", err)
	}
	// Opaque to the client: nothing readable may leak into a value the app
	// stores and logs.
	for _, leak := range []string{in.Email, in.ZitadelSessionID, in.ZitadelSessionToken, in.TenantID} {
		if contains(sealed, leak) {
			t.Fatalf("sealed token leaks %q in plaintext", leak)
		}
	}

	out, err := m.OpenPending(sealed)
	if err != nil {
		t.Fatalf("OpenPending: %v", err)
	}
	if out.UID != in.UID || out.Email != in.Email || out.TenantID != in.TenantID {
		t.Fatalf("identity did not round-trip: %+v", out)
	}
	if out.ZitadelSessionID != in.ZitadelSessionID || out.ZitadelSessionToken != in.ZitadelSessionToken {
		t.Fatalf("zitadel session handle did not round-trip: %+v", out)
	}
}

// Tampering must not be distinguishable from any other rejection, and must
// certainly not yield a usable Pending.
func TestOpenPending_RejectsTampered(t *testing.T) {
	m := testManager(t)
	sealed, err := m.SealPending(Pending{UID: "u1", TenantID: "t1", Email: "a@b.test"})
	if err != nil {
		t.Fatal(err)
	}

	bad := []byte(sealed)
	bad[len(bad)-1] ^= 'x'
	if _, err := m.OpenPending(string(bad)); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("tampered token: got %v, want ErrInvalidSession", err)
	}
	if _, err := m.OpenPending("not-base64!!"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("garbage token: got %v, want ErrInvalidSession", err)
	}
	if _, err := m.OpenPending(""); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("empty token: got %v, want ErrInvalidSession", err)
	}
}

// A challenge must not outlive its window — otherwise a leaked token stays
// usable indefinitely against a code the user may never have used.
func TestOpenPending_RejectsExpired(t *testing.T) {
	m := testManager(t)
	sealed, err := m.SealPending(Pending{
		UID: "u1", TenantID: "t1", Email: "a@b.test",
		ExpiresAt: time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenPending(sealed); !errors.Is(err, ErrExpiredSession) {
		t.Fatalf("expired token: got %v, want ErrExpiredSession", err)
	}
}

// A token sealed by a different key must not open: this is what stops one
// environment's challenge completing a login in another.
func TestOpenPending_RejectsForeignKey(t *testing.T) {
	sealed, err := testManager(t).SealPending(Pending{UID: "u1", TenantID: "t1"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewManager(Config{
		EncryptKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CookieName: "m8_session", Domain: "mark8ly.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.OpenPending(sealed); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("foreign-key token: got %v, want ErrInvalidSession", err)
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
