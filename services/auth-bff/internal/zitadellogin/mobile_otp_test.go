package zitadellogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeCodeVerifier struct {
	gotEmail, gotCode string
	err               error
}

func (f *fakeCodeVerifier) VerifyCode(_ context.Context, email, code string) error {
	f.gotEmail, f.gotCode = email, code
	return f.err
}

type fakePendingStore struct {
	sealed string
	opened *PendingLogin
	err    error
}

func (f *fakePendingStore) SealPendingLogin(PendingLogin) (string, error) { return f.sealed, nil }
func (f *fakePendingStore) OpenPendingLogin(string) (*PendingLogin, error) {
	return f.opened, f.err
}

func otpHandler(t *testing.T, cv *fakeCodeVerifier, ps *fakePendingStore, zitURL string, zitClient *http.Client) *Handler {
	t.Helper()
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	return NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	}).WithTokenIssuer(NewTokenExchanger(zitURL, testClientID, testClientPlaceholder, zitClient),
		"https://admin.mark8ly.com/auth/callback", "proj-1").
		WithStepUp(cv, ps)
}

func zitadelTokenAndAuthorize(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/v2/authorize" {
			w.Header().Set("Location", "https://admin.mark8ly.com/login?authRequest=V2_resume")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3599}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The happy path: a correct code resumes the login the pending token
// describes and yields tokens.
func TestMobileOTPVerify_CorrectCodeIssuesTokens(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	cv := &fakeCodeVerifier{}
	ps := &fakePendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := otpHandler(t, cv, ps, zit.URL, zit.Client())

	rec := httptest.NewRecorder()
	h.mobileOTPVerify(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/otp/verify",
		strings.NewReader(`{"pending_token":"sealed","code":"123456"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body["data"].(map[string]any)
	if data["access_token"] != "AT" {
		t.Fatalf("want tokens, got %v", body)
	}

	// THE binding property. The email the code is checked against must come
	// from the sealed token, never from the request body — otherwise a
	// caller holding their own valid code could complete someone else's
	// login by naming a different address.
	if cv.gotEmail != "a@b.test" {
		t.Fatalf("code verified against %q, want the email from the sealed token", cv.gotEmail)
	}
	if cv.gotCode != "123456" {
		t.Fatalf("code = %q", cv.gotCode)
	}
}

// A body-supplied email must be ignored entirely, even when present.
func TestMobileOTPVerify_IgnoresEmailInTheBody(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	cv := &fakeCodeVerifier{}
	ps := &fakePendingStore{opened: &PendingLogin{
		UID: "u1", Email: "victim@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := otpHandler(t, cv, ps, zit.URL, zit.Client())

	rec := httptest.NewRecorder()
	h.mobileOTPVerify(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/otp/verify",
		strings.NewReader(`{"pending_token":"sealed","code":"123456","email":"attacker@b.test"}`)))

	if cv.gotEmail != "victim@b.test" {
		t.Fatalf("a body-supplied email must never be used; got %q", cv.gotEmail)
	}
	_ = rec
}

// A wrong code must not mint anything, and must not reveal whether the
// pending token or the code was the problem.
func TestMobileOTPVerify_WrongCodeIssuesNothing(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	cv := &fakeCodeVerifier{err: errors.New("bad code")}
	ps := &fakePendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "sid", ZitadelSessionToken: "stok",
	}}
	h := otpHandler(t, cv, ps, zit.URL, zit.Client())

	rec := httptest.NewRecorder()
	h.mobileOTPVerify(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/otp/verify",
		strings.NewReader(`{"pending_token":"sealed","code":"000000"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Fatal("a rejected code must not yield tokens")
	}
}

// An expired or forged pending token is refused before the code is even
// checked — there is no identity to check it against.
func TestMobileOTPVerify_BadPendingTokenRefusedBeforeCodeCheck(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	cv := &fakeCodeVerifier{}
	ps := &fakePendingStore{err: errors.New("invalid session")}
	h := otpHandler(t, cv, ps, zit.URL, zit.Client())

	rec := httptest.NewRecorder()
	h.mobileOTPVerify(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/otp/verify",
		strings.NewReader(`{"pending_token":"forged","code":"123456"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if cv.gotCode != "" {
		t.Fatal("the code must not be checked when the pending token is unusable")
	}
}

// A pending token with no Zitadel session cannot be resumed — that is a
// server-side inconsistency, not a user error, and must not read as a bad
// code.
func TestMobileOTPVerify_MissingSessionHandleIsNotAuthFailure(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	cv := &fakeCodeVerifier{}
	ps := &fakePendingStore{opened: &PendingLogin{UID: "u1", Email: "a@b.test", TenantID: "t1"}}
	h := otpHandler(t, cv, ps, zit.URL, zit.Client())

	rec := httptest.NewRecorder()
	h.mobileOTPVerify(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/otp/verify",
		strings.NewReader(`{"pending_token":"sealed","code":"123456"}`)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (not 401)", rec.Code)
	}
}

// The login call must hand back the state the challenge resumes from.
// Without it the app receives email_otp_required and has no way to finish —
// a dead end that looks exactly like a working login.
func TestMobileLogin_EmailOTPReturnsAPendingToken(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	ps := &fakePendingStore{sealed: "sealed-value"}

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	}).WithTokenIssuer(NewTokenExchanger(zit.URL, testClientID, testClientPlaceholder, zit.Client()),
		"https://admin.mark8ly.com/auth/callback", "proj-1").
		WithStepUp(&fakeCodeVerifier{}, ps)

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body["data"].(map[string]any)
	if data["email_otp_required"] != true {
		t.Fatalf("want email_otp_required, got %v", body)
	}
	if data["pending_token"] != "sealed-value" {
		t.Fatalf("want a pending_token to resume from, got %v", data["pending_token"])
	}
	if _, leaked := data["access_token"]; leaked {
		t.Fatal("no token may be issued while the step-up is outstanding")
	}
}

// The WEB path must not gain a pending_token: it resumes from the cookie,
// and emitting the sealed value there would put a Zitadel session handle
// into a browser response for no reason.
func TestWebLogin_EmailOTPDoesNotReturnAPendingToken(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	}).WithTokenIssuer(NewTokenExchanger(zit.URL, testClientID, testClientPlaceholder, zit.Client()),
		"https://admin.mark8ly.com/auth/callback", "proj-1").
		WithStepUp(&fakeCodeVerifier{}, &fakePendingStore{sealed: "sealed-value"})

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body["data"].(map[string]any)
	if _, present := data["pending_token"]; present {
		t.Fatal("the web path resumes from its cookie and must not receive a sealed step-up token")
	}
}
