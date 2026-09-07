package session

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256

func newTestManager(t *testing.T, opts ...func(*Config)) *Manager {
	t.Helper()
	cfg := Config{
		CookieName: "m8_test",
		Domain:     ".mark8ly.com",
		Secure:     true,
		EncryptKey: testKey,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// ─────────────────────────────────────────────────────────────────────────
// Construction
// ─────────────────────────────────────────────────────────────────────────

func TestNewManager_RejectsBadKeyLength(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"too short", "abc"},
		{"15 bytes", "0123456789abcde"},
		{"17 bytes", "0123456789abcdef0"},
		{"33 bytes", "0123456789abcdef0123456789abcdef0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewManager(Config{
				CookieName: "x",
				EncryptKey: tc.key,
			})
			if err == nil {
				t.Errorf("expected error for key %q", tc.name)
			}
		})
	}
}

func TestNewManager_AcceptsAES128_192_256(t *testing.T) {
	cases := []int{16, 24, 32}
	for _, n := range cases {
		key := strings.Repeat("a", n)
		_, err := NewManager(Config{
			CookieName: "x",
			EncryptKey: key,
		})
		if err != nil {
			t.Errorf("NewManager with %d-byte key: %v", n, err)
		}
	}
}

// encryptForTest AES-GCM encrypts arbitrary raw JSON the same way encode
// does, without going through json.Marshal(Session{...}) — used to
// simulate a cookie shaped by an older version of Session that never
// had an auth_context field at all.
func encryptForTest(t *testing.T, m *Manager, plaintextJSON string) string {
	t.Helper()
	nonce := make([]byte, m.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	ciphertext := m.gcm.Seal(nonce, nonce, []byte(plaintextJSON), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext)
}

// ─────────────────────────────────────────────────────────────────────────
// Round-trip
// ─────────────────────────────────────────────────────────────────────────

func TestMintAndRead_RoundTrip(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()

	original := Session{
		UID:      "user-123",
		Email:    "user@example.com",
		TenantID: "MP-Internal-test",
	}
	if err := m.Mint(w, original); err != nil {
		t.Fatalf("Mint: %v", err)
	}

	// Take the Set-Cookie header from the response and feed it back into a request.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	got, err := m.Read(req)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("Read returned nil session")
	}
	if got.UID != original.UID {
		t.Errorf("UID = %q, want %q", got.UID, original.UID)
	}
	if got.Email != original.Email {
		t.Errorf("Email = %q, want %q", got.Email, original.Email)
	}
	if got.TenantID != original.TenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, original.TenantID)
	}
}

// TestEncodeSession_RoundTripsAuthContext exercises the exported
// EncodeSession -> decode path directly (not LoadFromValue — it does
// not exist) and asserts AuthContext survives the round trip.
func TestEncodeSession_RoundTripsAuthContext(t *testing.T) {
	m := newTestManager(t)

	original := Session{
		UID:         "user-123",
		TenantID:    "tenant-1",
		AuthContext: "break_glass",
	}
	encoded, err := m.EncodeSession(original)
	if err != nil {
		t.Fatalf("EncodeSession: %v", err)
	}

	got, err := m.decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AuthContext != "break_glass" {
		t.Errorf("AuthContext = %q, want %q", got.AuthContext, "break_glass")
	}
}

// TestDecode_OldShapedCookieDefaultsAuthContextEmpty proves backward
// compatibility: a cookie encoded before AuthContext existed (plain
// JSON with no auth_context key) still decodes, with AuthContext =="".
func TestDecode_OldShapedCookieDefaultsAuthContextEmpty(t *testing.T) {
	m := newTestManager(t)

	// Simulate a pre-AuthContext cookie by encrypting JSON with the
	// field entirely absent, the way json.Marshal(Session{...}) would
	// have produced before this field existed.
	oldShaped := `{"uid":"user-123","email":"u@example.com","tenant_id":"tenant-1","iat":"2026-01-01T00:00:00Z","exp":"2099-01-01T00:00:00Z"}`
	encoded := encryptForTest(t, m, oldShaped)

	got, err := m.decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AuthContext != "" {
		t.Errorf("AuthContext = %q, want empty on old-shaped cookie", got.AuthContext)
	}
	if got.UID != "user-123" {
		t.Errorf("UID = %q, want user-123", got.UID)
	}
}

func TestRead_ReturnsNilWhenCookieMissing(t *testing.T) {
	m := newTestManager(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	got, err := m.Read(req)
	if err != nil {
		t.Errorf("error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestRead_RejectsTamperedCookie(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()
	if err := m.Mint(w, Session{UID: "u1"}); err != nil {
		t.Fatal(err)
	}

	// Tamper: flip a character of the encoded value. Using a literal 'X'
	// here flaked ~1/64 runs when the original base64-encoded cookie
	// happened to start with 'X' (no-op tamper → valid cookie → no
	// error). Pick a substitute byte that's guaranteed different from
	// whatever's currently there.
	c := w.Result().Cookies()[0]
	tampered := *c
	orig := c.Value[0]
	sub := byte('X')
	if orig == sub {
		sub = 'Y'
	}
	tampered.Value = string(sub) + c.Value[1:]

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&tampered)

	if _, err := m.Read(req); err == nil {
		t.Error("Read should reject tampered cookie")
	}
}

func TestRead_RejectsExpiredCookie(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()

	expired := Session{
		UID:       "user-1",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour), // already expired
	}
	if err := m.Mint(w, expired); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}

	_, err := m.Read(req)
	if err != ErrExpiredSession {
		t.Errorf("err = %v, want ErrExpiredSession", err)
	}
}

func TestMint_RejectsEmptyUID(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()
	if err := m.Mint(w, Session{}); err == nil {
		t.Error("Mint should reject session with empty UID")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Cookie attributes (the auth-bug fixes)
// ─────────────────────────────────────────────────────────────────────────

// auth-bug #1: Domain must be set to enable cross-subdomain visibility.
func TestMint_SetsDomainAttribute(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()
	_ = m.Mint(w, Session{UID: "u"})

	c := w.Result().Cookies()[0]
	// Go's http.Cookie strips the leading dot per RFC 6265 §4.1.2.3.
	// Modern browsers treat ".mark8ly.com" and "mark8ly.com" identically
	// for cross-subdomain visibility, so the normalized form is correct
	// and the cookie is still readable from {tenant}.mark8ly.com.
	if c.Domain != "mark8ly.com" {
		t.Errorf("Domain = %q, want mark8ly.com (auth-bug #1 regression)", c.Domain)
	}
}

// auth-bug #4: SameSite must be Lax, NOT Strict, so the OAuth callback
// can carry the cookie back across the redirect.
func TestMint_SetsSameSiteLax(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()
	_ = m.Mint(w, Session{UID: "u"})

	c := w.Result().Cookies()[0]
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax (auth-bug #4 regression)", c.SameSite)
	}
}

// auth-bug #6: Secure flag must respect dev (HTTP) vs prod (HTTPS).
func TestMint_SecureFlagRespectsConfig(t *testing.T) {
	prod := newTestManager(t)
	w := httptest.NewRecorder()
	_ = prod.Mint(w, Session{UID: "u"})
	if !w.Result().Cookies()[0].Secure {
		t.Error("Secure should be true when configured")
	}

	dev := newTestManager(t, func(c *Config) { c.Secure = false })
	w2 := httptest.NewRecorder()
	_ = dev.Mint(w2, Session{UID: "u"})
	if w2.Result().Cookies()[0].Secure {
		t.Error("Secure should be false in dev to allow HTTP cookies")
	}
}

func TestMint_HttpOnlyAlwaysSet(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()
	_ = m.Mint(w, Session{UID: "u"})

	if !w.Result().Cookies()[0].HttpOnly {
		t.Error("HttpOnly should always be set; XSS must not read the cookie")
	}
}

func TestClear_OverwritesCookieWithMaxAgeMinusOne(t *testing.T) {
	m := newTestManager(t)
	w := httptest.NewRecorder()
	m.Clear(w)

	c := w.Result().Cookies()[0]
	if c.MaxAge != -1 {
		t.Errorf("Clear MaxAge = %d, want -1", c.MaxAge)
	}
	// Domain/SameSite must match the original cookie or the browser sets
	// a SECOND cookie instead of deleting the first. Go normalizes the
	// leading dot per RFC 6265 §4.1.2.3.
	if c.Domain != "mark8ly.com" {
		t.Errorf("Clear Domain = %q, want mark8ly.com", c.Domain)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("Clear SameSite = %v, want Lax", c.SameSite)
	}
}
