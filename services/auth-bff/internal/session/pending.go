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

	// Device context, carried so the challenge that completes the login
	// can write the session registry row for the device it actually
	// started on. Recomputing the fingerprint at verify time would risk
	// a User-Agent that shifted mid-flow never being recorded, which
	// would challenge that browser on every future sign-in.
	Fingerprint string `json:"fp,omitempty"`
	Device      string `json:"dev,omitempty"`

	// Zitadel session handle, carried ONLY on the mobile step-up path
	// (mark8ly#686). A native client has no cookie to resume from, and
	// after the challenge the server must re-finalize this session to
	// obtain an authorization code and exchange it for a bearer token.
	//
	// This is the same handle the TOTP step-up already returns to callers
	// (see zitadellogin's totp_required response) — the difference is that
	// here it is sealed rather than sent in the clear, and bound to the
	// uid/email/tenant above, which is what stops a challenge minted for
	// one account completing another's login.
	//
	// Empty on the browser path: the web resumes from the pending cookie
	// and never needs it.
	ZitadelSessionID    string `json:"zsid,omitempty"`
	ZitadelSessionToken string `json:"zst,omitempty"`
	IPAddress           string `json:"ip,omitempty"`
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

	encoded, err := m.SealPending(p)
	if err != nil {
		return err
	}

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

// SealPending encrypts a Pending into an opaque, URL-safe string.
//
// The cookie path and the mobile path share this so there is exactly one
// pending-state format and one place its crypto lives. A native client
// receives the result as a value it cannot read and hands back verbatim.
func (m *Manager) SealPending(p Pending) (string, error) {
	if p.UID == "" || p.TenantID == "" {
		return "", errors.New("session: pending requires UID and TenantID")
	}
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(PendingTTL)
	}
	plaintext, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("session: pending marshal: %w", err)
	}
	nonce := make([]byte, m.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("session: pending nonce: %w", err)
	}
	ciphertext := m.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// OpenPending reverses SealPending. Tampered input is ErrInvalidSession and
// an elapsed one ErrExpiredSession, mirroring the cookie taxonomy — a
// caller must never be able to tell a forged token from an expired one by
// the error alone.
func (m *Manager) OpenPending(value string) (*Pending, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
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
	return m.OpenPending(c.Value)
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
