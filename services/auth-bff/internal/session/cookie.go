// Package session implements encrypted cookie sessions for auth-bff.
//
// Sessions are AES-GCM encrypted JWTs stored in a single cookie. No Redis,
// no DB lookup on every request. The cookie itself IS the session.
//
// Bug fixes from docs/planning/auth-bugs.md:
//
//   - #1 cookie domain: Domain is set to ".mark8ly.com" (the parent of
//     {tenant}-admin.mark8ly.com and {tenant}.mark8ly.com) so the cookie
//     is readable across subdomains. Configurable so dev can use
//     "localhost" without the leading dot.
//
//   - #4 SameSite: set to Lax, NOT Strict. Strict prevents the OAuth
//     callback from carrying the cookie back across the redirect dance.
//
//   - #6 Secure flag: set ONLY when serving over HTTPS. Hard-coding
//     Secure=true in dev over plain HTTP causes the browser to set the
//     cookie but never send it back, looking like "logged in then
//     immediately logged out."
//
// The cookie payload is intentionally small (UID, tenant, exp, iat). Roles
// and permissions live in OpenFGA tuples and are looked up per-request.
// This keeps the cookie under the 4KB limit (auth-bug #5 from the writeup
// — high-permission users blew the cookie size).
package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Session is the validated payload of a session cookie.
type Session struct {
	UID       string    `json:"uid"`
	Email     string    `json:"email"`
	TenantID  string    `json:"tenant_id"` // mark8ly workspace tenant
	// StoreID is the currently active store under TenantID. Phase
	// Q.2 introduced this alongside `/auth/switch-store` so a
	// merchant with multiple stores can switch without leaving
	// admin. Empty on pre-Phase-Q.2 cookies — the admin falls
	// back to the first store under the current tenant when blank.
	StoreID   string    `json:"store_id,omitempty"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

// IsExpired returns true if the session is past its exp.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// Manager mints, parses, validates, and clears session cookies.
type Manager struct {
	cookieName string
	domain     string
	secure     bool
	maxAge     time.Duration
	gcm        cipher.AEAD
}

// Config holds Manager dependencies.
type Config struct {
	// CookieName: e.g. "m8_session"
	CookieName string
	// Domain: ".mark8ly.com" for prod, "localhost" for dev. The leading dot
	// is required for cross-subdomain visibility in some browsers.
	Domain string
	// Secure: true on HTTPS, false in HTTP dev. NOT a security knob — it's
	// a "does the cookie get sent at all" knob in dev.
	Secure bool
	// MaxAge is the session lifetime. Defaults to 7 days if zero.
	MaxAge time.Duration
	// EncryptKey is the AES-GCM key. MUST be exactly 16, 24, or 32 bytes
	// (AES-128, AES-192, AES-256). Validated at construction time.
	EncryptKey string
}

// NewManager constructs a session Manager.
//
// EncryptKey must be 16, 24, or 32 bytes. We use the raw byte length —
// callers should pass a high-entropy string from a secret manager, NOT
// a derived passphrase. (For dev, the loaded fake key is 32 bytes.)
func NewManager(cfg Config) (*Manager, error) {
	if cfg.CookieName == "" {
		return nil, fmt.Errorf("session: CookieName is required")
	}
	if cfg.EncryptKey == "" {
		return nil, fmt.Errorf("session: EncryptKey is required")
	}
	keyBytes := []byte(cfg.EncryptKey)
	if l := len(keyBytes); l != 16 && l != 24 && l != 32 {
		return nil, fmt.Errorf("session: EncryptKey must be 16, 24, or 32 bytes (got %d)", l)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("session: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("session: gcm: %w", err)
	}

	maxAge := cfg.MaxAge
	if maxAge == 0 {
		maxAge = 7 * 24 * time.Hour
	}

	return &Manager{
		cookieName: cfg.CookieName,
		domain:     cfg.Domain,
		secure:     cfg.Secure,
		maxAge:     maxAge,
		gcm:        gcm,
	}, nil
}

// Mint creates a session, serializes + encrypts it, and writes the cookie
// onto the response.
func (m *Manager) Mint(w http.ResponseWriter, s Session) error {
	if s.UID == "" {
		return errors.New("session: UID is required")
	}
	now := time.Now()
	if s.IssuedAt.IsZero() {
		s.IssuedAt = now
	}
	if s.ExpiresAt.IsZero() {
		s.ExpiresAt = now.Add(m.maxAge)
	}

	encoded, err := m.encode(s)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    encoded,
		Path:     "/",
		Domain:   m.domain,
		MaxAge:   int(m.maxAge.Seconds()),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode, // auth-bug #4 fix: Lax not Strict
	})
	return nil
}

// Read parses and validates the session cookie from a request. Returns
// nil and no error if the cookie is missing (caller decides if that's an
// error). Returns nil + ErrInvalidSession if the cookie is malformed,
// tampered, or expired.
func (m *Manager) Read(r *http.Request) (*Session, error) {
	c, err := r.Cookie(m.cookieName)
	if err != nil {
		// http.ErrNoCookie is the only error r.Cookie returns.
		return nil, nil
	}

	s, err := m.decode(c.Value)
	if err != nil {
		return nil, ErrInvalidSession
	}
	if s.IsExpired() {
		return nil, ErrExpiredSession
	}
	return s, nil
}

// Clear writes a max-age=-1 cookie to the response, telling the browser
// to delete it. The Domain/Path/SameSite must match the original cookie
// or the browser will set a SECOND cookie instead of deleting.
func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		Domain:   m.domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Sentinel errors.
var (
	ErrInvalidSession = errors.New("session: invalid or tampered cookie")
	ErrExpiredSession = errors.New("session: cookie expired")
)

// encode JSON-marshals the session, encrypts with AES-GCM, returns base64.
func (m *Manager) encode(s Session) (string, error) {
	plaintext, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("session: marshal: %w", err)
	}

	nonce := make([]byte, m.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("session: nonce: %w", err)
	}

	// AES-GCM appends the auth tag automatically. Output: nonce || ciphertext.
	ciphertext := m.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// decode is the inverse of encode. Any tampering with the cookie value
// (changing claims, modifying timestamps) fails the AEAD auth-tag check.
func (m *Manager) decode(encoded string) (*Session, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) < m.gcm.NonceSize() {
		return nil, errors.New("session: ciphertext too short")
	}

	nonce, ciphertext := raw[:m.gcm.NonceSize()], raw[m.gcm.NonceSize():]
	plaintext, err := m.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var s Session
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
