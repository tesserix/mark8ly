package autologin

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/deviceguard"
	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
)

// stubDevices reports a fixed new/known verdict.
type stubDevices struct {
	isNew bool
	err   error
	calls int
}

func (s *stubDevices) Evaluate(_ context.Context, _ deviceguard.Login) (bool, error) {
	s.calls++
	return s.isNew, s.err
}

// stubIssuer records the OTP challenges that were issued.
type stubIssuer struct {
	mu     sync.Mutex
	emails []string
	err    error
}

func (s *stubIssuer) IssueChallenge(_ context.Context, email, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.emails = append(s.emails, email)
	return nil
}

func (s *stubIssuer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.emails)
}

func newOTPService(t *testing.T, devices DeviceEvaluator, issuer ChallengeIssuer) *Service {
	t.Helper()
	sm, err := session.NewManager(session.Config{
		CookieName: "m8_test", Domain: "localhost", Secure: false, EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-1", "tenant-uuid-1")

	return NewService(Config{
		FGA: fgaFake, Sessions: sm, Policy: fastPolicy,
		Devices: devices, EmailOTP: issuer,
	})
}

func loginCtx() zitadellogin.LoginContext {
	return zitadellogin.LoginContext{
		UID:       "user-1",
		Email:     "u@e.com",
		TenantID:  "tenant-uuid-1",
		UserAgent: "Mozilla/5.0 (Macintosh) Chrome/128",
		Device:    "Chrome on macOS",
		IPAddress: "203.0.113.9",
		Country:   "IN",
	}
}

func cookieNames(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range w.Result().Cookies() {
		out[c.Name] = c.Value
	}
	return out
}

// TestCompleteForProvider_NewDevice_RequiresOTP is the core of the feature: an
// unrecognised device gets a pending cookie and a mailed code, never a
// live session.
func TestCompleteForProvider_NewDevice_RequiresOTP(t *testing.T) {
	issuer := &stubIssuer{}
	svc := newOTPService(t, &stubDevices{isNew: true}, issuer)
	w := httptest.NewRecorder()

	res, err := svc.CompleteForProvider(context.Background(), w, loginCtx())
	if err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if !res.EmailOTPRequired {
		t.Error("EmailOTPRequired = false, want true for an unrecognised device")
	}
	if issuer.count() != 1 {
		t.Fatalf("issued %d challenges, want 1", issuer.count())
	}
	if issuer.emails[0] != "u@e.com" {
		t.Errorf("challenge sent to %q", issuer.emails[0])
	}

	cookies := cookieNames(t, w)
	if _, ok := cookies[session.PendingCookieName]; !ok {
		t.Error("no pending cookie was written")
	}
	if v, ok := cookies["m8_test"]; ok && v != "" {
		t.Error("a full session cookie was minted before the code was verified")
	}
}

// TestCompleteForProvider_KnownDevice_SkipsOTP keeps the common case one step.
func TestCompleteForProvider_KnownDevice_SkipsOTP(t *testing.T) {
	issuer := &stubIssuer{}
	svc := newOTPService(t, &stubDevices{isNew: false}, issuer)
	w := httptest.NewRecorder()

	res, err := svc.CompleteForProvider(context.Background(), w, loginCtx())
	if err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if res.EmailOTPRequired {
		t.Error("EmailOTPRequired = true, want false for a known device")
	}
	if issuer.count() != 0 {
		t.Errorf("issued %d challenges, want 0", issuer.count())
	}
	if _, ok := cookieNames(t, w)["m8_test"]; !ok {
		t.Error("expected a full session cookie for a known device")
	}
}

// TestCompleteForProvider_NoIssuer_SkipsOTP preserves the pre-feature behaviour so
// an unconfigured environment still logs people in.
func TestCompleteForProvider_NoIssuer_SkipsOTP(t *testing.T) {
	svc := newOTPService(t, &stubDevices{isNew: true}, nil)
	w := httptest.NewRecorder()

	res, err := svc.CompleteForProvider(context.Background(), w, loginCtx())
	if err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if res.EmailOTPRequired {
		t.Error("EmailOTPRequired = true with no issuer wired")
	}
	if _, ok := cookieNames(t, w)["m8_test"]; !ok {
		t.Error("expected a full session cookie when OTP is not configured")
	}
}

// TestCompleteForProvider_IssuerFailure_FailsClosed asserts we do not fall through
// to a live session when the code could not be sent. Falling open here
// would mean an attacker who can break the mail path bypasses the gate
// entirely.
func TestCompleteForProvider_IssuerFailure_FailsClosed(t *testing.T) {
	issuer := &stubIssuer{err: errors.New("resend down")}
	svc := newOTPService(t, &stubDevices{isNew: true}, issuer)
	w := httptest.NewRecorder()

	_, err := svc.CompleteForProvider(context.Background(), w, loginCtx())
	if err == nil {
		t.Fatal("expected an error when the challenge could not be sent")
	}
	if !errors.Is(err, ErrChallengeSendFail) {
		t.Errorf("err = %v, want ErrChallengeSendFail", err)
	}
	if _, ok := cookieNames(t, w)["m8_test"]; ok {
		t.Error("a session cookie was minted despite the challenge failing")
	}
}

// TestCompleteForProvider_DeviceCheckFailure_RequiresOTP asserts an unreadable
// device history is treated as "new device" rather than waved through.
func TestCompleteForProvider_DeviceCheckFailure_RequiresOTP(t *testing.T) {
	issuer := &stubIssuer{}
	svc := newOTPService(t, &stubDevices{isNew: true, err: errors.New("db down")}, issuer)
	w := httptest.NewRecorder()

	res, err := svc.CompleteForProvider(context.Background(), w, loginCtx())
	if err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if !res.EmailOTPRequired {
		t.Error("a failed device lookup must fall back to challenging")
	}
}

// TestCompleteForProvider_PendingCarriesFingerprint asserts the pending cookie has
// what the verify step needs to record the device as known.
func TestCompleteForProvider_PendingCarriesFingerprint(t *testing.T) {
	svc := newOTPService(t, &stubDevices{isNew: true}, &stubIssuer{})
	w := httptest.NewRecorder()

	if _, err := svc.CompleteForProvider(context.Background(), w, loginCtx()); err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}

	sm, err := session.NewManager(session.Config{
		CookieName: "m8_test", Domain: "localhost", Secure: false, EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	p, err := sm.ReadPending(r)
	if err != nil {
		t.Fatalf("ReadPending: %v", err)
	}
	if p == nil {
		t.Fatal("no pending session found")
	}
	want := deviceguard.Fingerprint("Mozilla/5.0 (Macintosh) Chrome/128")
	if p.Fingerprint != want {
		t.Errorf("Fingerprint = %q, want %q", p.Fingerprint, want)
	}
	if p.Device != "Chrome on macOS" {
		t.Errorf("Device = %q", p.Device)
	}
	if p.IPAddress != "203.0.113.9" {
		t.Errorf("IPAddress = %q", p.IPAddress)
	}
	if p.Email != "u@e.com" {
		t.Errorf("Email = %q", p.Email)
	}
}

// TestCompleteForProvider_MFATakesPriority: a user with TOTP enrolled gets the TOTP
// challenge, not an emailed code. Two challenges for one login would be
// noise, and TOTP is the stronger factor.
func TestCompleteForProvider_MFATakesPriority(t *testing.T) {
	issuer := &stubIssuer{}
	sm, err := session.NewManager(session.Config{
		CookieName: "m8_test", Domain: "localhost", Secure: false, EncryptKey: testKey,
	})
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}
	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-1", "tenant-uuid-1")

	svc := NewService(Config{
		FGA: fgaFake, Sessions: sm, Policy: fastPolicy,
		MFA:      stubMFA{enabled: true},
		Devices:  &stubDevices{isNew: true},
		EmailOTP: issuer,
	})
	w := httptest.NewRecorder()

	res, err := svc.CompleteForProvider(context.Background(), w, loginCtx())
	if err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if !res.MFARequired {
		t.Error("MFARequired = false, want true")
	}
	if res.EmailOTPRequired {
		t.Error("EmailOTPRequired = true, want false when TOTP already gates the login")
	}
	if issuer.count() != 0 {
		t.Errorf("issued %d email challenges, want 0", issuer.count())
	}
}

type stubMFA struct{ enabled bool }

func (s stubMFA) IsEnabled(_ context.Context, _ string) (bool, error) { return s.enabled, nil }

// TestCompleteForProvider_AlertStillSentOnChallenge: the sign-in attempt itself is
// what the account holder needs to hear about, so the device alert must
// not wait on the code being verified.
func TestCompleteForProvider_AlertStillSentOnChallenge(t *testing.T) {
	devices := &stubDevices{isNew: true}
	svc := newOTPService(t, devices, &stubIssuer{})
	w := httptest.NewRecorder()

	if _, err := svc.CompleteForProvider(context.Background(), w, loginCtx()); err != nil {
		t.Fatalf("CompleteForProvider: %v", err)
	}
	if devices.calls != 1 {
		t.Errorf("deviceguard called %d times, want 1 (it owns alert dispatch)", devices.calls)
	}
}

// TestCompleteForProvider_RateLimitSurvivesWrap is the narrow error-chain
// property the shared gauntlet must hold: errors.Is has to see through the
// ErrChallengeSendFail wrap down to the emailotp sentinel, so a caller can
// still tell "we could not send" from "you asked too often".
func TestCompleteForProvider_RateLimitSurvivesWrap(t *testing.T) {
	issuer := &stubIssuer{err: emailotp.ErrRateLimited}
	svc := newOTPService(t, &stubDevices{isNew: true}, issuer)

	_, err := svc.CompleteForProvider(context.Background(), httptest.NewRecorder(), loginCtx())
	if err == nil {
		t.Fatal("expected an error when the challenge is rate limited")
	}
	if !errors.Is(err, ErrChallengeSendFail) {
		t.Error("errors.Is(err, ErrChallengeSendFail) = false")
	}
	if !errors.Is(err, emailotp.ErrRateLimited) {
		t.Error("errors.Is(err, emailotp.ErrRateLimited) = false")
	}
}

var _ = time.Now
