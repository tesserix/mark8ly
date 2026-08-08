package loginotp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/session"
)

const testKey = "0123456789abcdef0123456789abcdef"

// memStore is an in-memory emailotp.Store.
type memStore struct {
	mu   sync.Mutex
	recs []emailotp.Record
}

func (m *memStore) Insert(_ context.Context, r emailotp.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append(m.recs, r)
	return nil
}

func (m *memStore) Latest(_ context.Context, email string) (*emailotp.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.recs) - 1; i >= 0; i-- {
		if m.recs[i].Email == email {
			cp := m.recs[i]
			return &cp, nil
		}
	}
	return nil, emailotp.ErrNoChallenge
}

func (m *memStore) IncrementAttempts(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID == id {
			m.recs[i].Attempts++
		}
	}
	return nil
}

func (m *memStore) Consume(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID == id {
			if m.recs[i].ConsumedAt != nil {
				return emailotp.ErrAlreadyUsed
			}
			m.recs[i].ConsumedAt = &at
		}
	}
	return nil
}

func (m *memStore) CountSince(_ context.Context, email string, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.recs {
		if r.Email == email && r.CreatedAt.After(since) {
			n++
		}
	}
	return n, nil
}

// capMailer records the codes that would have been emailed.
type capMailer struct {
	mu    sync.Mutex
	codes []string
	err   error
}

func (c *capMailer) SendLoginCode(_ context.Context, _, code string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.codes = append(c.codes, code)
	return nil
}

func (c *capMailer) last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.codes) == 0 {
		return ""
	}
	return c.codes[len(c.codes)-1]
}

type recordingRegistry struct {
	mu      sync.Mutex
	created []CreateParams
	err     error
}

func (r *recordingRegistry) CreateSession(_ context.Context, p CreateParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.created = append(r.created, p)
	return nil
}

func (r *recordingRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.created)
}

type harness struct {
	gate     *Gate
	mailer   *capMailer
	registry *recordingRegistry
	mgr      *session.Manager
	router   *gin.Engine
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	otpSvc, err := emailotp.NewService(emailotp.Config{
		Store:  &memStore{},
		Pepper: "0123456789abcdefghij",
	})
	if err != nil {
		t.Fatalf("emailotp: %v", err)
	}
	mailer := &capMailer{}
	gate := NewGate(otpSvc, mailer, emailotp.DefaultTTL)

	mgr, err := session.NewManager(session.Config{
		CookieName: "m8_test", Domain: "localhost", Secure: false, EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	registry := &recordingRegistry{}

	r := gin.New()
	NewHandler(Config{Gate: gate, Sessions: mgr, Registry: registry}).Register(r.Group("/auth"))

	return &harness{gate: gate, mailer: mailer, registry: registry, mgr: mgr, router: r}
}

// pendingCookie mints a pending cookie the way autologin would.
func (h *harness) pendingCookie(t *testing.T, email string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	err := h.mgr.MintPending(w, session.Pending{
		UID: "user-1", Email: email, TenantID: "tenant-1",
		Fingerprint: "fp-abc", Device: "Chrome on macOS", IPAddress: "203.0.113.9",
	})
	if err != nil {
		t.Fatalf("MintPending: %v", err)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == session.PendingCookieName {
			return c
		}
	}
	t.Fatal("no pending cookie minted")
	return nil
}

func (h *harness) post(t *testing.T, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

// TestVerify_HappyPath is the whole point: the right code turns a pending
// login into a real session.
func TestVerify_HappyPath(t *testing.T) {
	h := newHarness(t)
	pc := h.pendingCookie(t, "u@e.com")

	if err := h.gate.IssueChallenge(context.Background(), "u@e.com", "203.0.113.9"); err != nil {
		t.Fatalf("IssueChallenge: %v", err)
	}
	code := h.mailer.last()
	if code == "" {
		t.Fatal("no code was mailed")
	}

	w := h.post(t, "/auth/otp/verify", `{"code":"`+code+`"}`, pc)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["authenticated"] != true {
		t.Errorf("authenticated = %v", got["authenticated"])
	}

	var sessionSet, pendingCleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "m8_test" && c.Value != "" {
			sessionSet = true
		}
		if c.Name == session.PendingCookieName && c.MaxAge < 0 {
			pendingCleared = true
		}
	}
	if !sessionSet {
		t.Error("no session cookie was minted")
	}
	if !pendingCleared {
		t.Error("pending cookie was not cleared")
	}
}

// TestVerify_RecordsDeviceAsKnown is what stops the user being challenged
// on every single sign-in from the same laptop.
func TestVerify_RecordsDeviceAsKnown(t *testing.T) {
	h := newHarness(t)
	pc := h.pendingCookie(t, "u@e.com")
	_ = h.gate.IssueChallenge(context.Background(), "u@e.com", "")

	h.post(t, "/auth/otp/verify", `{"code":"`+h.mailer.last()+`"}`, pc)

	if h.registry.count() != 1 {
		t.Fatalf("registry rows = %d, want 1", h.registry.count())
	}
	got := h.registry.created[0]
	if got.Fingerprint != "fp-abc" {
		t.Errorf("Fingerprint = %q, want fp-abc — the device would stay unknown forever", got.Fingerprint)
	}
	if got.UserID != "user-1" || got.TenantID != "tenant-1" {
		t.Errorf("identity not carried through: %+v", got)
	}
	if got.Device != "Chrome on macOS" {
		t.Errorf("Device = %q", got.Device)
	}
}

// TestVerify_WrongCode must not mint anything.
func TestVerify_WrongCode(t *testing.T) {
	h := newHarness(t)
	pc := h.pendingCookie(t, "u@e.com")
	_ = h.gate.IssueChallenge(context.Background(), "u@e.com", "")

	w := h.post(t, "/auth/otp/verify", `{"code":"000000"}`, pc)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "m8_test" && c.Value != "" {
			t.Error("a session cookie was minted for a wrong code")
		}
	}
	if h.registry.count() != 0 {
		t.Error("a session row was recorded for a wrong code")
	}
}

// TestVerify_NoPendingCookie: a code alone must never mint a session. This
// is the check that keeps email OTP a second factor rather than a
// standalone way in.
func TestVerify_NoPendingCookie(t *testing.T) {
	h := newHarness(t)
	_ = h.gate.IssueChallenge(context.Background(), "u@e.com", "")

	w := h.post(t, "/auth/otp/verify", `{"code":"`+h.mailer.last()+`"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "m8_test" && c.Value != "" {
			t.Fatal("a session was minted with no pending login in flight")
		}
	}
}

// TestVerify_CodeFromAnotherAddress closes the cross-account hole: a code
// legitimately issued to one address must not complete another's login.
func TestVerify_CodeFromAnotherAddress(t *testing.T) {
	h := newHarness(t)
	pc := h.pendingCookie(t, "victim@e.com")

	// Attacker requests a code for their own address and tries it against
	// the victim's pending session.
	if err := h.gate.IssueChallenge(context.Background(), "attacker@e.com", ""); err != nil {
		t.Fatalf("IssueChallenge: %v", err)
	}

	w := h.post(t, "/auth/otp/verify", `{"code":"`+h.mailer.last()+`"}`, pc)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — a code must be bound to its address", w.Code)
	}
}

// TestVerify_Replay asserts a consumed code cannot be used twice.
func TestVerify_Replay(t *testing.T) {
	h := newHarness(t)
	_ = h.gate.IssueChallenge(context.Background(), "u@e.com", "")
	code := h.mailer.last()

	first := h.post(t, "/auth/otp/verify", `{"code":"`+code+`"}`, h.pendingCookie(t, "u@e.com"))
	if first.Code != http.StatusOK {
		t.Fatalf("first verify status = %d, want 200", first.Code)
	}

	second := h.post(t, "/auth/otp/verify", `{"code":"`+code+`"}`, h.pendingCookie(t, "u@e.com"))
	if second.Code == http.StatusOK {
		t.Fatal("a consumed code was accepted a second time")
	}
}

// TestVerify_MalformedBody covers the input edge.
func TestVerify_MalformedBody(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"empty code", `{"code":""}`},
		{"missing field", `{}`},
		{"not json", `{`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			w := h.post(t, "/auth/otp/verify", tt.body, h.pendingCookie(t, "u@e.com"))
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// TestResend_IssuesNewCode lets a user who lost the first mail try again.
func TestResend_IssuesNewCode(t *testing.T) {
	h := newHarness(t)
	pc := h.pendingCookie(t, "u@e.com")

	w := h.post(t, "/auth/otp/resend", `{}`, pc)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if h.mailer.last() == "" {
		t.Error("no code was mailed on resend")
	}
}

// TestResend_NoPending refuses to mail anything without a login in
// flight — otherwise the endpoint is an open mail relay keyed by email.
func TestResend_NoPending(t *testing.T) {
	h := newHarness(t)

	w := h.post(t, "/auth/otp/resend", `{}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if h.mailer.last() != "" {
		t.Error("a code was mailed with no pending login")
	}
}

// TestVerify_ResponseNeverEchoesCode guards the log-leak path.
func TestVerify_ResponseNeverEchoesCode(t *testing.T) {
	h := newHarness(t)
	pc := h.pendingCookie(t, "u@e.com")
	_ = h.gate.IssueChallenge(context.Background(), "u@e.com", "")
	code := h.mailer.last()

	w := h.post(t, "/auth/otp/verify", `{"code":"`+code+`"}`, pc)

	if strings.Contains(w.Body.String(), code) {
		t.Errorf("response echoes the code: %s", w.Body.String())
	}
}

// TestVerify_RegistryFailureStillLogsIn: the session cookie is already the
// source of truth. Losing the audit row must not lock a verified user out.
func TestVerify_RegistryFailureStillLogsIn(t *testing.T) {
	h := newHarness(t)
	h.registry.err = errors.New("db down")
	pc := h.pendingCookie(t, "u@e.com")
	_ = h.gate.IssueChallenge(context.Background(), "u@e.com", "")

	w := h.post(t, "/auth/otp/verify", `{"code":"`+h.mailer.last()+`"}`, pc)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the registry write failing", w.Code)
	}
}

// TestGate_MailerFailurePropagates so autologin can fail the login closed.
func TestGate_MailerFailurePropagates(t *testing.T) {
	otpSvc, err := emailotp.NewService(emailotp.Config{Store: &memStore{}, Pepper: "0123456789abcdefghij"})
	if err != nil {
		t.Fatalf("emailotp: %v", err)
	}
	gate := NewGate(otpSvc, &capMailer{err: errors.New("resend down")}, time.Minute)

	if err := gate.IssueChallenge(context.Background(), "u@e.com", ""); err == nil {
		t.Fatal("expected the mailer failure to surface")
	}
}
