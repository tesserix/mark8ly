package zitadellogin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// tokenServer stands in for Zitadel's /oauth/v2/token.
func tokenServer(t *testing.T) *TokenExchanger {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3599}`))
	}))
	t.Cleanup(srv.Close)
	return NewTokenExchanger(srv.URL, testClientID, testClientPlaceholder, srv.Client())
}

// The mobile route differs from the web one in EXACTLY one respect: a
// completed login yields tokens instead of a callback_url. Everything
// before that — credential handling, subject re-resolution, the gauntlet —
// must be the same code, which is why this asserts the gauntlet still ran
// with the Zitadel-resolved identity rather than anything from the body.
func TestMobileLogin_CompleteReturnsTokensNotCallbackURL(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	var gotUID, gotEmail, gotTenant string
	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotUID, gotEmail, gotTenant = lc.UID, lc.Email, lc.TenantID
		return CompleteResult{}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1")

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data["access_token"] != "AT" || data["refresh_token"] != "RT" {
		t.Fatalf("want tokens, got %v", body)
	}
	// A mobile client cannot use a callback_url for anything, and leaking
	// a live authorization code into an app is pointless risk.
	if _, leaked := body["callback_url"]; leaked {
		t.Fatalf("callback_url must not be returned to a mobile client: %v", body)
	}
	if gotUID == "" || gotEmail != "a@b.test" || gotTenant != "t1" {
		t.Fatalf("gauntlet did not run with the Zitadel-resolved identity: uid=%q email=%q tenant=%q", gotUID, gotEmail, gotTenant)
	}
}

// A fresh install is ALWAYS an unrecognised device, so this is the common
// first-login path, not an edge case. It must answer with the step-up
// shape and NO tokens — handing tokens over here would defeat the OTP.
func TestMobileLogin_EmailOTPRequiredReturnsNoTokens(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1").
		// Step-up must be wired: without it the client would be handed a
		// challenge it cannot complete, and login now refuses instead —
		// see TestMobileLogin_EmailOTPWithoutStepUpConfiguredRefuses.
		WithStepUp(&fakeCodeVerifier{}, &fakePendingStore{sealed: "sealed-value"})

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body["data"].(map[string]any)
	if data["email_otp_required"] != true {
		t.Fatalf("want email_otp_required, got %v", body)
	}
	if _, ok := data["access_token"]; ok {
		t.Fatal("tokens must NOT be issued while a step-up is outstanding")
	}
}

// The MFA gate must behave identically to the web route: no session, no
// tokens, no finalize.
func TestMobileLogin_TOTPGateIssuesNoTokens(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		t.Fatal("complete() must not run at the MFA gate")
		return CompleteResult{}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1")

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["totp_required"] != true {
		t.Fatalf("want totp_required, got %v", body)
	}
	if fin.Load() {
		t.Fatal("finalize must not run at the MFA gate")
	}
}

// Without a configured issuer the mobile route must fail loudly rather
// than fall back to callback_url, which the client cannot use and which
// would look like a working login right up until the first API call 401s.
func TestMobileLogin_NoIssuerConfiguredFailsLoudly(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	})

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "callback_url") {
		t.Fatal("must not degrade to callback_url when no issuer is configured")
	}
}

// The web route must be untouched by any of this.
func TestWebLoginStillReturnsCallbackURLWhenAnIssuerIsConfigured(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1")

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["callback_url"] == nil {
		t.Fatalf("the web route must still answer with callback_url, got %v", body)
	}
	if _, leaked := body["access_token"]; leaked {
		t.Fatal("the web route must not start issuing tokens")
	}
}

// A mobile caller has no browser to be redirected through
// /oauth/v2/authorize, so it cannot obtain an auth_request_id. The server
// mints one — but ONLY for the mobile route: a web caller creating a
// second auth request would orphan the flow its user is actually in.
func TestMobileLogin_MintsItsOwnAuthRequestWhenNoneSupplied(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	var authorizeHits int
	zit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/authorize":
			authorizeHits++
			w.Header().Set("Location", "https://admin.mark8ly.com/login?authRequest=V2_minted")
			w.WriteHeader(http.StatusFound)
		default:
			_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3599}`))
		}
	}))
	defer zit.Close()

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	}).WithTokenIssuer(NewTokenExchanger(zit.URL, testClientID, testClientPlaceholder, zit.Client()),
		"https://admin.mark8ly.com/auth/callback", "proj-1")

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if authorizeHits != 1 {
		t.Fatalf("authorize called %d times, want exactly 1", authorizeHits)
	}
}

// The web route must still REQUIRE an auth_request_id — its browser
// already has one, and minting a second would orphan the live flow.
func TestWebLogin_StillRequiresAnAuthRequestID(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1")

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a web login with no auth_request_id", rec.Code)
	}
}

// A step-up the client cannot finish is a dead end that reads as a working
// login, so it fails loudly at the server instead.
func TestMobileLogin_EmailOTPWithoutStepUpConfiguredRefuses(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1")

	rec := httptest.NewRecorder()
	h.mobileLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/mobile/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when the step-up cannot be completed", rec.Code)
	}
}
