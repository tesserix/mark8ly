package zitadellogin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

// testGoogleIDPID is the fixed Google IDP id every fake/fixture in this file
// uses. A Handler under test must be configured with
// WithGoogleIDPID(testGoogleIDPID) for idp/finish to accept an identity
// carrying it — see the idp-id pinning check in idpFinish.
const testGoogleIDPID = "idp-1"

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
		case r.Method == http.MethodPost && r.URL.Path == "/v2/idp_intents":
			// StartIDPIntent. The fixed authUrl lets idp/start tests assert
			// on an exact value without needing per-test fixtures.
			w.Write([]byte(`{"authUrl":"https://idp.example.test/auth?state=intent-1"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			// RetrieveIDPIntent. Always resolves to the same linked user
			// ("u1") that SessionFactors/UserEmail below also resolve to, so
			// idp/finish tests exercise the same identity throughout.
			// idpId is testGoogleIDPID — a Handler under test must be built
			// with WithGoogleIDPID(testGoogleIDPID) to accept this fixture.
			w.Write([]byte(`{"userId":"u1","idpInformation":{"idpId":"` + testGoogleIDPID + `","rawInformation":{"email":"a@b.test","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(`{"sessionToken":"tok-2"}`))
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyJSON))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":` + methodsJSON + `}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			// UserEmail / UserEmailVerified: the fixed handler resolves
			// email (and its verified flag, for the customer password
			// login gate) from Zitadel, not from the request body, so
			// every handler test needs this. isVerified:true so existing
			// login/totp tests built on this fixture keep passing the new
			// gate — tests of the gate itself use their own fixture.
			w.Write([]byte(`{"user":{"human":{"email":{"email":"a@b.test","isVerified":true}}}}`))
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
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		return CompleteResult{}, nil
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
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotUID, gotEmail, gotTenant = lc.UID, lc.Email, lc.TenantID
		return CompleteResult{}, nil
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
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		return CompleteResult{}, nil
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
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		return CompleteResult{}, nil
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
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		got = lc
		return CompleteResult{}, nil
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

// TestTotpIgnoresAClientSuppliedLoginNameAndResolvesTheRealUser pins the fix
// for a spoofing defect: login_name on /zitadel/totp is never checked against
// anything (the password check that verifies login_name happens on the
// earlier /zitadel/login call, against a session this caller might not even
// own). Without resolving the email from Zitadel, anyone with valid
// credentials of their own could submit an arbitrary login_name here and walk
// away with a session cookie, an audit event, and a mailed sign-in code
// addressed to a victim's email of their choosing.
func TestTotpIgnoresAClientSuppliedLoginNameAndResolvesTheRealUser(t *testing.T) {
	var fin atomic.Bool
	factorsWithTOTP := `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"},"totp":{"verifiedAt":"2026-09-03T01:01:00Z"}}}}`
	c := fakeZitadelHandler(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsWithTOTP, &fin)

	var got LoginContext
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		got = lc
		return CompleteResult{}, nil
	})

	rec := httptest.NewRecorder()
	h.totp(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/totp",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"attacker-chosen@evil.test","session_id":"s1","session_token":"tok-1","code":"123456","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Email == "attacker-chosen@evil.test" {
		t.Fatalf("LoginContext.Email = %q — a client-supplied, unverified login_name reached the gauntlet on the TOTP step", got.Email)
	}
	if got.Email != "a@b.test" {
		t.Fatalf("LoginContext.Email = %q, want the email resolved from Zitadel (a@b.test), regardless of what login_name was submitted", got.Email)
	}
}

// TestLoginCompleteWithMFARequiredDoesNotReturnCallbackURL pins the fix for
// CompleteForProvider discarding the step-up state: if auth-bff's own MFA
// gate fires inside the gauntlet, the response must say so instead of
// answering with a callback_url that would tell the browser the login is
// finished while a step-up is still outstanding.
func TestLoginCompleteWithMFARequiredDoesNotReturnCallbackURL(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		return CompleteResult{MFARequired: true}, nil
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
	if _, ok := body["callback_url"]; ok {
		t.Fatalf("body = %v carries callback_url while auth-bff's own MFA step-up is outstanding", body)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["mfa_required"] != true {
		t.Fatalf("body = %v, want data.mfa_required = true", body)
	}
	if data["email_otp_required"] != false {
		t.Fatalf("body = %v, want data.email_otp_required = false", body)
	}
}

// TestLoginCompleteWithEmailOTPRequiredDoesNotReturnCallbackURL is the
// email-OTP twin of the MFA case above: a new-device challenge must also
// suppress callback_url, not just MFA.
func TestLoginCompleteWithEmailOTPRequiredDoesNotReturnCallbackURL(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
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
	if _, ok := body["callback_url"]; ok {
		t.Fatalf("body = %v carries callback_url while the email-OTP step-up is outstanding", body)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["email_otp_required"] != true {
		t.Fatalf("body = %v, want data.email_otp_required = true", body)
	}
	if data["mfa_required"] != false {
		t.Fatalf("body = %v, want data.mfa_required = false", body)
	}
}

// TestRegisterResolvesTheGinClientIPIntoLoginContext exercises the actual
// Register -> withClientIP plumbing through a real gin router, rather than
// calling h.login/h.totp directly as every other test in this file does. No
// prior test routed through Register, so a break in that wiring (IPAddress
// silently landing as "") would have gone unnoticed — the same shape of gap
// as the Fingerprint("") defect this phase already found and fixed.
func TestRegisterResolvesTheGinClientIPIntoLoginContext(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	var gotIP string
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotIP = lc.IPAddress
		return CompleteResult{}, nil
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r.Group("/auth"))

	req := httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.42")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotIP == "" {
		t.Fatal("IPAddress reaching CompleteFunc is empty — the Register -> withClientIP plumbing is broken; " +
			"the email-OTP limiter would key on a constant for every Zitadel login")
	}
	if gotIP != "203.0.113.42" {
		t.Fatalf("IPAddress = %q, want the gin-resolved client IP from X-Forwarded-For", gotIP)
	}
}

// mustAllowlist builds a ReturnURLAllowlist from known-good entries for
// tests that only care about idp/start's downstream behaviour, not
// NewReturnURLAllowlist's own validation (that is returnurl_test.go's job).
func mustAllowlist(t *testing.T, hosts, suffixes []string) ReturnURLAllowlist {
	t.Helper()
	a, err := NewReturnURLAllowlist(hosts, suffixes)
	if err != nil {
		t.Fatalf("NewReturnURLAllowlist(%v, %v): %v", hosts, suffixes, err)
	}
	return a
}

// policyForceMFALocalOnly is not shared with sufficiency_test.go: it exists
// only to prove idp/finish threads federated=true into CompleteIfSufficient,
// which login() cannot exercise (a password login is never federated).
const policyForceMFALocalOnly = `{"policy":{"passwordCheckLifetime":"0s","forceMfaLocalOnly":true}}`

func TestIDPStartRejectsDisallowedReturnURL(t *testing.T) {
	// unreachableZitadel fails the test outright if Zitadel is ever called:
	// the allowlist must reject this candidate before StartIDPIntent runs.
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).
		WithReturnURLAllowlist(mustAllowlist(t, []string{"admin.mark8ly.com"}, nil))

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/start",
		strings.NewReader(`{"return_url":"https://evil.example.com/x"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestIDPStartRejectsMissingFields(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).
		WithReturnURLAllowlist(mustAllowlist(t, []string{"admin.mark8ly.com"}, nil))

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/start", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestIDPStartRefusesWithoutAConfiguredGoogleIDPID pins the config item: the
// idp id is no longer a compiled-in constant, so a Handler built without
// WithGoogleIDPID must fail closed with a clean 500 rather than sending
// Zitadel an empty idpId.
func TestIDPStartRefusesWithoutAConfiguredGoogleIDPID(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).
		WithReturnURLAllowlist(mustAllowlist(t, []string{"admin.mark8ly.com"}, nil))

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/start",
		strings.NewReader(`{"return_url":"https://admin.mark8ly.com/auth/idp/finish"}`)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
}

func TestIDPStartReturnsAuthURLForAnAllowedReturnURL(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewHandler(c, nil).
		WithReturnURLAllowlist(mustAllowlist(t, []string{"admin.mark8ly.com"}, nil)).
		WithGoogleIDPID("idp-1")

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/start",
		strings.NewReader(`{"return_url":"https://admin.mark8ly.com/auth/idp/finish"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["auth_url"] != "https://idp.example.test/auth?state=intent-1" {
		t.Fatalf("body = %v", body)
	}
}

// TestIDPFinishIgnoresATamperedUserQueryParam is the core anti-takeover
// assertion for this endpoint: Zitadel's success redirect carries a `user`
// value that is attacker-controlled (it rides in a URL the browser
// followed), and the frontend may forward whatever it received in that
// field. Two requests, identical except for that field, must resolve to the
// exact same identity and mint a session for the exact same subject — proof
// that `user` is never consulted.
func TestIDPFinishIgnoresATamperedUserQueryParam(t *testing.T) {
	for _, tamperedUser := range []string{"", "some-other-victim-user-id", "u1"} {
		t.Run("user="+tamperedUser, func(t *testing.T) {
			var fin atomic.Bool
			c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

			var gotUID string
			h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
				gotUID = lc.UID
				return CompleteResult{}, nil
			}).WithGoogleIDPID(testGoogleIDPID)

			body := `{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1","user":"` + tamperedUser + `"}`
			rec := httptest.NewRecorder()
			h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish", strings.NewReader(body)))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			// SessionFactors in the fake always resolves to "u1" regardless
			// of what the request's user field said — this is the fixed
			// identity RetrieveIDPIntent + SessionFactors resolve, not
			// req.User.
			if gotUID != "u1" {
				t.Fatalf("complete() called with uid=%q, want u1 regardless of the tampered user field", gotUID)
			}
			var respBody map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if respBody["callback_url"] != "https://admin.mark8ly.com/auth/callback?code=c&state=s" {
				t.Fatalf("body = %v", respBody)
			}
		})
	}
}

func TestIDPFinishRejectsMissingFields(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewHandler(c, nil)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestIDPFinishRefusesUnlinkedIdentityWithUnverifiedEmail is the absolute
// rule from the security review: an unlinked federated identity may be
// attached to an account by email ONLY when the provider asserts that
// email is verified. false must refuse WITHOUT ever reaching the account
// lookup or creation calls — proven the unreachableZitadel way, not merely
// by asserting the response.
func TestIDPFinishRefusesUnlinkedIdentityWithUnverifiedEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"idp-1","userId":"ext-1","userName":"new.person@gmail.com","rawInformation":{"email":"new.person@gmail.com","email_verified":false}}}`))
		default:
			t.Errorf("must not be called for an unverified email: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishRefusesUnlinkedIdentityWithNoEmailClaim covers the other half
// of the same rule: a provider that omits the email claim entirely (not
// merely marks it false) must refuse identically — readRawEmail's
// default-false-on-absent behaviour (see idpintent_test.go) must never be
// mistaken for "probably fine" here.
func TestIDPFinishRefusesUnlinkedIdentityWithNoEmailClaim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"idp-1","userId":"ext-1","userName":"","rawInformation":{}}}`))
		default:
			t.Errorf("must not be called with no email claim: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishLinksAFirstTimeIdentityToAnExistingVerifiedAccount: when a
// verified email already belongs to an existing Zitadel user, this handler
// must attach the new Google identity to THAT account (via LinkIDPToUser)
// rather than register a second, disconnected one — and must complete
// sign-in as that existing user, not create anything new.
func TestIDPFinishLinksAFirstTimeIdentityToAnExistingVerifiedAccount(t *testing.T) {
	var fin, linkCalled, createCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/existing-1/links":
			linkCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			createCalled.Store(true)
			t.Error("must not create a new user when the existing account can be linked instead")
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyNoMFA))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":["AUTHENTICATION_METHOD_TYPE_PASSWORD"]}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			w.Write([]byte(`{"user":{"human":{"email":{"email":"person@gmail.com"}}}}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			fin.Store(true)
			w.Write([]byte(`{"callbackUrl":"https://admin.mark8ly.com/auth/callback?code=c&state=s"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(`{"session":{"factors":{"user":{"id":"existing-1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-04T01:00:00Z"}}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var gotUID string
	h := NewHandler(New(srv.URL, "pat", srv.Client()), func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotUID = lc.UID
		return CompleteResult{}, nil
	}).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")

	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !linkCalled.Load() {
		t.Fatal("LinkIDPToUser (POST /v2/users/existing-1/links) was never called")
	}
	if createCalled.Load() {
		t.Fatal("a new user must not be created when the existing account was linked")
	}
	if gotUID != "existing-1" {
		t.Fatalf("complete() called with uid=%q, want existing-1", gotUID)
	}
	if !fin.Load() {
		t.Fatal("finalize was never called")
	}
}

// TestIDPFinishRejectsAnIntentFromAnUnexpectedIDP is the fix for the review
// finding that this endpoint never checked which IDP an intent came from.
// The instance can (and does) carry more than one IDP (e.g. a separately
// added Apple IDP); an intent retrieved from anything other than the
// configured Google IDP must be refused before ANY lookup, link, or
// creation call runs — otherwise a weaker or attacker-influenced provider's
// email_verified claim would be trusted exactly like Google's.
func TestIDPFinishRejectsAnIntentFromAnUnexpectedIDP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			// A DIFFERENT idp id than testGoogleIDPID — simulating an
			// intent started against some other IDP on the instance (e.g.
			// Apple), carrying its own verified-email claim for the same
			// victim address.
			w.Write([]byte(`{"idpInformation":{"idpId":"some-other-idp","userId":"ext-1","userName":"victim@merchant.com","rawInformation":{"email":"victim@merchant.com","email_verified":true}}}`))
		default:
			t.Errorf("must not look up, link, or create anything for an intent from an unexpected idp: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishRefusesAnAmbiguousEmailMatchAndCreatesNothing is the fix for
// the review finding that FindUserByVerifiedEmail took the first match: two
// orgs on a shared instance can each hold a verified copy of the same
// email, so more than one match within even a single scoped search must be
// treated as an ambiguity to refuse, never a result to pick from — and
// certainly never grounds to fall through to creation.
func TestIDPFinishRefusesAnAmbiguousEmailMatchAndCreatesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[
				{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}},
				{"userId":"existing-2","human":{"email":{"email":"person@gmail.com","isVerified":true}}}
			]}`))
		default:
			t.Errorf("must not link or create anything on an ambiguous match: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishRefusesWhenNoAdminAccountMatchesAndCreatesNothing is Finding
// 4's ruling: the merchant path is LINK-ONLY. A verified, unlinked identity
// with no existing account match must be refused — never registered.
// Merchant authorization is FGA tenant membership keyed by user id, so a
// freshly created user is guaranteed to fail that gauntlet; creating one
// here would be pure garbage generation. CreateHumanUserWithIDPLink must
// never be reached from this endpoint.
func TestIDPFinishRefusesWhenNoAdminAccountMatchesAndCreatesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"108234...google-sub","userName":"new.person@gmail.com","rawInformation":{"email":"new.person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		default:
			t.Errorf("the merchant path must never create a user: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishRefusesToLinkAnExistingAccountWithoutAVerifiedEmail is the
// security boundary for the link path specifically: an existing-account
// match must never be linked when the CURRENT sign-in attempt's identity
// carries an unverified or absent email, even if some other lookup could in
// principle find a matching account. Neither the link call nor the create
// call may be reached — FindUserByVerifiedEmail itself must not even run,
// since the email-verified gate is checked before it.
func TestIDPFinishRefusesToLinkAnExistingAccountWithoutAVerifiedEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"idp-1","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":false}}}`))
		default:
			t.Errorf("must not look up, link, or create anything for an unverified email: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishSecondSignInResolvesViaUserIDAfterLinking proves the point of
// linking at all: once linked to an existing account, a subsequent
// retrieve for the same provider identity comes back already resolved, so
// idpFinish signs in through the ordinary linked path and never calls
// LinkIDPToUser again.
func TestIDPFinishSecondSignInResolvesViaUserIDAfterLinking(t *testing.T) {
	var linked, fin atomic.Bool
	linkCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			if linked.Load() {
				w.Write([]byte(`{"userId":"existing-1","idpInformation":{"idpId":"idp-1","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
			} else {
				w.Write([]byte(`{"idpInformation":{"idpId":"idp-1","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/existing-1/links":
			linkCalls++
			linked.Store(true)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyNoMFA))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":["AUTHENTICATION_METHOD_TYPE_PASSWORD"]}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			w.Write([]byte(`{"user":{"human":{"email":{"email":"person@gmail.com"}}}}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			fin.Store(true)
			w.Write([]byte(`{"callbackUrl":"https://admin.mark8ly.com/auth/callback?code=c&state=s"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(`{"session":{"factors":{"user":{"id":"existing-1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-04T01:00:00Z"}}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var gotUIDs []string
	h := NewHandler(New(srv.URL, "pat", srv.Client()), func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotUIDs = append(gotUIDs, lc.UID)
		return CompleteResult{}, nil
	}).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")

	body := `{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`

	rec1 := httptest.NewRecorder()
	h.idpFinish(rec1, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish", strings.NewReader(body)))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first sign-in: status = %d, body = %s", rec1.Code, rec1.Body.String())
	}

	rec2 := httptest.NewRecorder()
	h.idpFinish(rec2, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish", strings.NewReader(body)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second sign-in: status = %d, body = %s", rec2.Code, rec2.Body.String())
	}

	if linkCalls != 1 {
		t.Fatalf("LinkIDPToUser was called %d times, want exactly 1 — the second sign-in must resolve via user_id, not link again", linkCalls)
	}
	if len(gotUIDs) != 2 || gotUIDs[0] != "existing-1" || gotUIDs[1] != "existing-1" {
		t.Fatalf("gotUIDs = %v, want [existing-1 existing-1]", gotUIDs)
	}
}

// TestIDPFinishSurfacesAnInvalidIntentWithoutLeakingZitadelBody mirrors
// TestLoginCollapsesUnknownUserAndWrongPasswordIntoOneAnswer's shape for the
// retrieve step: a bad/expired/already-consumed intent must answer a clean
// 401 without echoing Zitadel's own error body.
func TestIDPFinishSurfacesAnInvalidIntentWithoutLeakingZitadelBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"intent not found (COMMAND-2Ls8f)","details":[{"id":"COMMAND-2Ls8f"}]}`))
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "COMMAND-2Ls8f") {
		t.Fatalf("response leaks the Zitadel error id: %s", rec.Body.String())
	}
}

// TestIDPFinishIsExemptFromForceMFALocalOnly proves finish() threads
// federated=true into CompleteIfSufficient. Under forceMfaLocalOnly with no
// TOTP verified, a password login (federated=false) would hand off or
// demand a factor; a Google sign-in through this endpoint must complete.
func TestIDPFinishIsExemptFromForceMFALocalOnly(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFALocalOnly, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	completeCalled := false
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		return CompleteResult{}, nil
	}).WithGoogleIDPID(testGoogleIDPID)

	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !completeCalled {
		t.Fatal("complete() was not called — forceMfaLocalOnly must not apply to a federated Google sign-in")
	}
	if !fin.Load() {
		t.Fatal("finalize was not called — forceMfaLocalOnly must not apply to a federated Google sign-in")
	}
}

// TestIDPFinishWithoutTenantReturnsTenantRequiredAndDoesNotComplete is Task
// 1's core case: the admin app cannot know which tenant a Google identity
// belongs to until AFTER this retrieve/link/session-create sequence runs, so
// when workspace_tenant is absent, idpFinish must still do everything up to
// and including creating the Zitadel session — retrieve, pin the idp, verify
// the email, link to the existing account — and then hand back a
// tenant_required response instead of completing. finalize() and complete()
// must never run: CompleteIfSufficient's oidc/auth_requests call would mean
// this login front-ran a tenant it doesn't have yet, and complete() would
// mint an m8_session for one of possibly several tenants the user belongs
// to.
func TestIDPFinishWithoutTenantReturnsTenantRequiredAndDoesNotComplete(t *testing.T) {
	var fin, linkCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/existing-1/links":
			linkCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			fin.Store(true)
			t.Error("finalize must not run when workspace_tenant is absent")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("must not be called when deferring tenant selection: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	completeCalled := false
	h := NewHandler(New(srv.URL, "pat", srv.Client()), func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		t.Error("complete() must not run when workspace_tenant is absent")
		return CompleteResult{}, nil
	}).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")

	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !linkCalled.Load() {
		t.Fatal("LinkIDPToUser was never called — linking must still happen before deferring")
	}
	if completeCalled || fin.Load() {
		t.Fatal("complete()/finalize must not run when workspace_tenant is absent")
	}

	var respBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody["tenant_required"] != true {
		t.Fatalf("body = %v, want tenant_required=true", respBody)
	}
	if respBody["session_id"] != "s1" {
		t.Fatalf("body = %v, want session_id=s1", respBody)
	}
	if respBody["session_token"] != "tok-1" {
		t.Fatalf("body = %v, want session_token=tok-1", respBody)
	}
	if respBody["login_name"] != "person@gmail.com" {
		t.Fatalf("body = %v, want login_name=person@gmail.com (from the retrieved identity)", respBody)
	}
	if respBody["callback_url"] != nil {
		t.Fatalf("body = %v, must not carry a callback_url", respBody)
	}
}

// TestIDPFinishWithoutTenantIgnoresACallerSuppliedLoginName proves login_name
// in the tenant_required response comes from the retrieved identity, never
// from anything the caller could pass in the request body — the same
// discipline idpFinishRequest.User's doc comment already requires for the
// `user` field.
func TestIDPFinishWithoutTenantIgnoresACallerSuppliedLoginName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/existing-1/links":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		default:
			t.Errorf("must not be called: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","user":"attacker@evil.test"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var respBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody["login_name"] != "person@gmail.com" {
		t.Fatalf("body = %v, login_name must come from the retrieved identity, not the caller-supplied user field", respBody)
	}
}

// TestIDPFinishWithoutTenantStillRefusesUnverifiedEmail proves the deferred-
// tenant branch does not create a new bypass around the absolute
// verified-email gate: an unlinked identity with an unverified email must
// still refuse before any lookup, link, or session creation — exactly as it
// does when workspace_tenant is present.
func TestIDPFinishWithoutTenantStillRefusesUnverifiedEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"new.person@gmail.com","rawInformation":{"email":"new.person@gmail.com","email_verified":false}}}`))
		default:
			t.Errorf("must not be called for an unverified email: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishWithoutTenantStillRejectsAnIntentFromAnUnexpectedIDP proves
// the idp-pinning gate runs before the deferred-tenant branch too.
func TestIDPFinishWithoutTenantStillRejectsAnIntentFromAnUnexpectedIDP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"some-other-idp","userId":"ext-1","userName":"victim@merchant.com","rawInformation":{"email":"victim@merchant.com","email_verified":true}}}`))
		default:
			t.Errorf("must not look up, link, or create anything for an intent from an unexpected idp: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPFinishWithoutTenantStillRefusesNoAdminAccountAndReturnsNoSession is
// the most important negative case for this task: the merchant path stays
// link-only regardless of whether workspace_tenant was supplied, and that
// refusal must happen BEFORE the new deferred-tenant branch — a merchant
// with no admin account must never receive a session, tenant_required or
// otherwise.
func TestIDPFinishWithoutTenantStillRefusesNoAdminAccountAndReturnsNoSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"108234...google-sub","userName":"new.person@gmail.com","rawInformation":{"email":"new.person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			t.Error("must not create a session when no admin account matches")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("the merchant path must never create a user or session: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
	var respBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if respBody["session_id"] != nil || respBody["session_token"] != nil || respBody["tenant_required"] != nil {
		t.Fatalf("body = %v, must not carry any session/tenant_required fields", respBody)
	}
}

// A Zitadel 400 on the intent -> session exchange must NEVER surface as
// `invalid_credentials`.
//
// Observed in production 2026-09-06: a merchant whose Google identity was
// correctly linked (one Zitadel user, verified email, one Google IDP link)
// was told to check their details, and the log said "login rejected: bad
// credentials" — on a flow where no credential is presented at all. The
// cause was this call site sharing respondSessionCreateError, which maps
// CreatePasswordSession's errors, combined with do() turning any Zitadel 400
// into ErrBadCredentials.
//
// google-sign-in-admin.ts states the rule this broke: no outcome of this
// flow may imply the Google credential itself was wrong, because it never
// is. `invalid_intent` is the honest answer — the intent was refused.
func TestIDPFinishReportsARefusedIntentExchangeAsInvalidIntent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/existing-1/links":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			// Zitadel refusing the exchange — a consumed or expired intent.
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":3,"message":"invalid intent","details":[{"id":"COMMAND-3M0fs"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "invalid_credentials") {
		t.Fatalf("a Google sign-in must never be reported as a credential failure, got %s", body)
	}
	if !strings.Contains(body, "invalid_intent") {
		t.Fatalf("error = %s, want invalid_intent", body)
	}
}

// The ALREADY-LINKED path must still resolve a user for the session.
//
// This is the case that failed in production: a merchant accepted an invite,
// set a password, and signed in with Google. Their Google identity was
// already linked, so `identity.ZitadelUserID` was set, the link block was
// skipped entirely — and the session was then created with no user check at
// all, which Zitadel refuses with COMMAND-Sfw3r (Errors.User.UserIDMissing).
//
// The link-creating path had a test; this one did not, and it is the one
// every returning Google user takes.
func TestIDPFinishSendsTheLinkedUserWhenTheIdentityIsAlreadyLinked(t *testing.T) {
	var sessionBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			// userId present => Zitadel already knows this identity.
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}},"userId":"linked-user-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			t.Error("must not search by email: the identity is already linked")
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			b, _ := io.ReadAll(r.Body)
			sessionBody = string(b)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).
		WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok"}`)))

	if !strings.Contains(sessionBody, `"userId":"linked-user-1"`) {
		t.Fatalf("session must be created for the linked user, body = %s", sessionBody)
	}
}
