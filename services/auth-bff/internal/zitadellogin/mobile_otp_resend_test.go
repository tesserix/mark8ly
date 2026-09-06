package zitadellogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeChallengeIssuer struct {
	gotEmail, gotIP string
	calls           int
	err             error
}

func (f *fakeChallengeIssuer) IssueChallenge(_ context.Context, email, ip string) error {
	f.calls++
	f.gotEmail, f.gotIP = email, ip
	return f.err
}

// resealingPendingStore hands back a DIFFERENT sealed value on every seal,
// which is what lets a test tell a re-sealed token apart from the one the
// caller sent in.
type resealingPendingStore struct {
	opened *PendingLogin
	err    error
	seals  int
	last   PendingLogin
}

func (f *resealingPendingStore) SealPendingLogin(p PendingLogin) (string, error) {
	f.seals++
	f.last = p
	return fmt.Sprintf("resealed-%d", f.seals), nil
}

func (f *resealingPendingStore) OpenPendingLogin(string) (*PendingLogin, error) {
	return f.opened, f.err
}

func resendHandler(t *testing.T, ci *fakeChallengeIssuer, ps PendingStore) *Handler {
	t.Helper()
	zit := zitadelTokenAndAuthorize(t)
	h := otpHandler(t, &fakeCodeVerifier{}, &fakePendingStore{}, zit.URL, zit.Client())
	h.pending = ps
	return h.WithChallengeIssuer(ci)
}

func resendRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/otp/resend", strings.NewReader(body))
	return withClientIP(r, "203.0.113.9")
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	data, _ := body["data"].(map[string]any)
	return data
}

// THE property this route exists for: a fresh code AND a fresh pending
// token, because the two expire together. Returning only a code would hand
// the merchant a correct code against an about-to-expire challenge.
func TestMobileOTPResend_IssuesACodeAndReturnsAFreshPendingToken(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	ps := &resealingPendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := resendHandler(t, ci, ps)

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"original"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	if data["sent"] != true {
		t.Fatalf("want sent:true, got %v", data)
	}
	fresh, _ := data["pending_token"].(string)
	if fresh == "" {
		t.Fatal("a resend with no fresh pending token leaves the client verifying against an about-to-expire challenge")
	}
	if fresh == "original" {
		t.Fatal("the returned pending token must differ from the one sent in")
	}
	// The re-seal must carry the SAME Zitadel session, or the next verify
	// has nothing to resume.
	if ps.last.ZitadelSessionID != "sid" || ps.last.ZitadelSessionToken != "stok" {
		t.Fatalf("re-seal lost the zitadel session: %+v", ps.last)
	}
	if ps.last.UID != "u1" || ps.last.TenantID != "t1" {
		t.Fatalf("re-seal lost the identity: %+v", ps.last)
	}
}

// The address is bound to the sealed token. Trusting a body-supplied one
// would turn this route into a way to mail a code anywhere.
func TestMobileOTPResend_IgnoresEmailInTheBody(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	ps := &resealingPendingStore{opened: &PendingLogin{
		UID: "u1", Email: "victim@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := resendHandler(t, ci, ps)

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"original","email":"attacker@b.test"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ci.gotEmail != "victim@b.test" {
		t.Fatalf("code mailed to %q; a body-supplied address must never be used", ci.gotEmail)
	}
}

// The limiter keys on the client IP, so the resolved IP has to reach it —
// reading a header directly here would key every proxied request the same.
func TestMobileOTPResend_PassesTheResolvedClientIP(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	ps := &resealingPendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := resendHandler(t, ci, ps)

	h.mobileOTPResend(httptest.NewRecorder(), resendRequest(`{"pending_token":"original"}`))

	if ci.gotIP != "203.0.113.9" {
		t.Fatalf("issuer saw ip %q, want the resolved client IP", ci.gotIP)
	}
}

// A spent budget gets its OWN code. Folded into a generic failure, the
// merchant is left tapping a button that will never work.
func TestMobileOTPResend_RateLimitedGetsItsOwnCode(t *testing.T) {
	ci := &fakeChallengeIssuer{err: fmt.Errorf("issue: %w", ErrChallengeRateLimited)}
	ps := &resealingPendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := resendHandler(t, ci, ps)

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"original"}`))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "rate_limited" {
		t.Fatalf("error = %v, want rate_limited", body["error"])
	}
	if ps.seals != 0 {
		t.Fatal("nothing was sent, so nothing may be re-sealed")
	}
}

// Any other issuer failure is ours, not the merchant's.
func TestMobileOTPResend_OtherIssuerFailureIsNotRateLimited(t *testing.T) {
	ci := &fakeChallengeIssuer{err: errors.New("smtp down")}
	ps := &resealingPendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := resendHandler(t, ci, ps)

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"original"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "rate_limited") {
		t.Fatal("a delivery failure must not read as a spent budget")
	}
}

// A forged or expired token is refused before anything is mailed — there
// is no address to mail to that we would trust.
func TestMobileOTPResend_BadPendingTokenMailsNothing(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	h := resendHandler(t, ci, &resealingPendingStore{err: errors.New("invalid session")})

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"forged"}`))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ci.calls != 0 {
		t.Fatal("no code may be issued for an unusable pending token")
	}
}

// An expired token opens to nothing rather than erroring in some stores;
// that must be refused identically to a forged one.
func TestMobileOTPResend_MissingPendingIsRefusedLikeAForgedOne(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	h := resendHandler(t, ci, &resealingPendingStore{})

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"expired"}`))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ci.calls != 0 {
		t.Fatal("no code may be issued without a usable challenge")
	}
}

// The internal-auth gate stands in front of this route exactly as it does
// its siblings: without it, auth-bff would publish an unauthenticated way
// to make it send mail.
func TestMobileOTPResend_RequiresInternalAuth(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	ps := &resealingPendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := resendHandler(t, ci, ps).WithInternalAuth("s3cret")

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"original"}`))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ci.calls != 0 {
		t.Fatal("an unauthorized caller must not be able to make us send mail")
	}
}

// No issuer wired means the route refuses loudly rather than answering
// sent:true for a code nobody mailed.
func TestMobileOTPResend_RefusesWhenUnconfigured(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	h := otpHandler(t, &fakeCodeVerifier{}, &fakePendingStore{}, zit.URL, zit.Client())

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{"pending_token":"original"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestMobileOTPResend_RequiresAPendingToken(t *testing.T) {
	ci := &fakeChallengeIssuer{}
	h := resendHandler(t, ci, &resealingPendingStore{})

	rec := httptest.NewRecorder()
	h.mobileOTPResend(rec, resendRequest(`{}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if ci.calls != 0 {
		t.Fatal("nothing to resend against; no code may be issued")
	}
}
