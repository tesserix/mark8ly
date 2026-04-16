package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"crypto/rand"
)

// PendingCookieName is the cookie that carries the half-authenticated
// state between auto-login (token verified) and MFA challenge (code
// verified). Scoped to the admin host only so it never leaks across
// other mark8ly subdomains.
const PendingCookieName = "m8_mfa_pending"

// PendingTTL is the window a user has to complete MFA after password
// or Google sign-in succeeds. 5 minutes is long enough for a sleepy
// laptop scan + type, short enough that an abandoned challenge can't
// be resumed hours later.
const PendingTTL = 5 * time.Minute

// Pending is the payload encoded into the MFA-pending cookie. It
// carries just enough to complete the challenge without re-running
// the GIP token verification.
type Pending struct {
	UID       string    `json:"uid"`
	Email     string    `json:"email"`
	TenantID  string    `json:"tenant_id"`
	ExpiresAt time.Time `json:"exp"`
}

// IsExpired reports whether the pending challenge has timed out.
func (p *Pending) IsExpired() bool { return time.Now().After(p.ExpiresAt) }

// MintPending writes the encrypted pending cookie onto the response.
// Reuses the Manager's AES-GCM but emits a distinct cookie so it
// can't be mistaken for a full session.
func (m *Manager) MintPending(w http.ResponseWriter, p Pending) error {
	if p.UID == "" || p.TenantID == "" {
		return errors.New("session: pending requires UID and TenantID")
	}
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(PendingTTL)
	}

	plaintext, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("session: pending marshal: %w", err)
	}
	nonce := make([]byte, m.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("session: pending nonce: %w", err)
	}
	ciphertext := m.gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.RawURLEncoding.EncodeToString(ciphertext)

	http.SetCookie(w, &http.Cookie{
		Name:     PendingCookieName,
		Value:    encoded,
		Path:     "/",
		Domain:   m.domain,
		MaxAge:   int(PendingTTL.Seconds()),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// ReadPending parses and validates the pending cookie from a request.
// Returns nil with no error when the cookie is absent; that's the
// normal "no challenge in flight" state. Tampered or expired cookies
// surface as ErrInvalidSession / ErrExpiredSession to mirror the
// primary session error taxonomy.
func (m *Manager) ReadPending(r *http.Request) (*Pending, error) {
	c, err := r.Cookie(PendingCookieName)
	if err != nil {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return nil, ErrInvalidSession
	}
	if len(raw) < m.gcm.NonceSize() {
		return nil, ErrInvalidSession
	}
	nonce, ct := raw[:m.gcm.NonceSize()], raw[m.gcm.NonceSize():]
	plain, err := m.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrInvalidSession
	}
	var p Pending
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, ErrInvalidSession
	}
	if p.IsExpired() {
		return nil, ErrExpiredSession
	}
	return &p, nil
}

// ClearPending writes a max-age=-1 cookie so the browser drops the
// pending state. Called after a successful challenge and after any
// hard failure that ends the attempt.
func (m *Manager) ClearPending(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     PendingCookieName,
		Value:    "",
		Path:     "/",
		Domain:   m.domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}
