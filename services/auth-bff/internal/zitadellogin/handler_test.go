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

func TestLoginCollapsesUnknownUserAndWrongPasswordIntoOneAnswer(t *testing.T) {
	// A different status or message for "no such user" is an account-
	// enumeration oracle. Both must look identical to the browser.
	for _, body := range []string{
		`{"code":3,"message":"Password is invalid (COMMAND-3M0fs)","details":[{"id":"COMMAND-3M0fs","failedAttempts":1}]}`,
		`{"code":5,"message":"User could not be found (QUERY-Dfbg2)","details":[{"id":"QUERY-Dfbg2"}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(body, "QUERY") {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			w.Write([]byte(body))
		}))
		defer srv.Close()
		h := NewHandler(New(srv.URL, "pat", srv.Client()), nil)
		rec := httptest.NewRecorder()
		h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
			strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "failedAttempts") || strings.Contains(rec.Body.String(), "not be found") {
			t.Fatalf("response leaks which half failed: %s", rec.Body.String())
		}
	}
}

// fakeZitadelHandler wires the same routes fakeZitadel (sufficiency_test.go)
// wires, plus /v2/sessions for CreatePasswordSession and PATCH for VerifyTOTP,
// so handler tests can drive the full login/totp path against one fake.
func fakeZitadelHandler(t *testing.T, policyJSON, methodsJSON, factorsJSON string, finalized *atomic.Bool) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(`{"sessionToken":"tok-2"}`))
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyJSON))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":` + methodsJSON + `}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			finalized.Store(true)
			w.Write([]byte(`{"callbackUrl":"https://admin.mark8ly.com/auth/callback?code=c&state=s"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(factorsJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}

// factorsPasswordOnly, policyNoMFA and policyForceMFA are shared with
// sufficiency_test.go.

func TestLoginFactorRequiredDoesNotMintSession(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)

	completeCalled := false
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) error {
		completeCalled = true
		return nil
	})

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["totp_required"] != true {
		t.Fatalf("body = %v, want totp_required: true", body)
	}
	if completeCalled {
		t.Fatal("complete() was called for OutcomeFactorRequired — a session must not be minted at the MFA gate")
	}
	if fin.Load() {
		t.Fatal("finalize was called for OutcomeFactorRequired")
	}
}

func TestLoginCompleteCallsCompleteAndReturnsCallbackURL(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	var gotUID, gotEmail, gotTenant string
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) error {
		gotUID, gotEmail, gotTenant = lc.UID, lc.Email, lc.TenantID
		return nil
	})

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["callback_url"] != "https://admin.mark8ly.com/auth/callback?code=c&state=s" {
		t.Fatalf("body = %v", body)
	}
	if gotUID != "u1" || gotEmail != "a@b.test" || gotTenant != "t1" {
		t.Fatalf("complete called with uid=%q email=%q tenant=%q", gotUID, gotEmail, gotTenant)
	}
}

func TestLoginHandoffReturnsHandoffURLAndDoesNotCallComplete(t *testing.T) {
	var fin atomic.Bool
	// Unrecognised factor type -> uncollectible -> OutcomeHandoff with a nil
	// error (ordinary uncertainty, not a finalize failure).
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_U2F"]`, factorsPasswordOnly, &fin)

	completeCalled := false
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) error {
		completeCalled = true
		return nil
	}).WithHostedLoginBaseURL("https://login.mark8ly.zitadel.cloud")

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, _ := body["handoff_url"].(string)
	if !strings.HasPrefix(got, "https://login.mark8ly.zitadel.cloud/ui/v2/login/login?authRequestID=") {
		t.Fatalf("handoff_url = %q", got)
	}
	if completeCalled {
		t.Fatal("complete() was called for OutcomeHandoff")
	}
	if fin.Load() {
		t.Fatal("finalize was called for OutcomeHandoff")
	}
}

func TestTotpWrongCodeReturns401WithoutLeakingZitadelBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":3,"message":"Invalid code (COMMAND-Sfeg2)","details":[{"id":"COMMAND-Sfeg2","failedAttempts":2}]}`))
	}))
	defer srv.Close()
	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil)

	rec := httptest.NewRecorder()
	h.totp(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/totp",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","session_id":"s1","session_token":"tok-1","code":"000000","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "failedAttempts") {
		t.Fatalf("response leaks the attempt counter: %s", rec.Body.String())
	}
}

func TestTotpCompleteCallsCompleteAndReturnsCallbackURL(t *testing.T) {
	var fin atomic.Bool
	factorsWithTOTP := `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"},"totp":{"verifiedAt":"2026-09-03T01:01:00Z"}}}}`
	c := fakeZitadelHandler(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsWithTOTP, &fin)

	completeCalled := false
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) error {
		completeCalled = true
		return nil
	})

	rec := httptest.NewRecorder()
	h.totp(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/totp",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","session_id":"s1","session_token":"tok-1","code":"123456","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !completeCalled {
		t.Fatal("complete() was not called after a successful TOTP check")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["callback_url"] == "" {
		t.Fatalf("body = %v", body)
	}
}

func TestLoginRejectsMissingFields(t *testing.T) {
	h := NewHandler(New("http://unused.invalid", "pat", nil), nil)
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestLoginPassesTheUserAgentThroughSoDeviceguardStillWorks pins the fix for
// a critical, invisible-to-tests defect: an earlier CompleteFunc shape had no
// way to carry request metadata to completeLogin at all, so
// deviceguard.Fingerprint would have hashed "" for every Zitadel login —
// a CONSTANT every user shares, silently collapsing new-device detection.
func TestLoginPassesTheUserAgentThroughSoDeviceguardStillWorks(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	const wantUA = "Mark8lyZitadelLoginTest/1.0 (distinctive-ua)"
	var got LoginContext
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) error {
		got = lc
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`))
	req.Header.Set("User-Agent", wantUA)

	rec := httptest.NewRecorder()
	h.login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.UserAgent != wantUA {
		t.Fatalf("LoginContext.UserAgent = %q, want %q — deviceguard fingerprints this; "+
			"empty would collapse every Zitadel user onto one fingerprint", got.UserAgent, wantUA)
	}
}

func TestHandoffURLEmptyWithoutConfiguredBase(t *testing.T) {
	h := NewHandler(New("http://unused.invalid", "pat", nil), nil)
	if got := h.handoffURL("V2_1"); got != "" {
		t.Fatalf("handoffURL = %q, want empty when no base configured", got)
	}
}
