package zitadellogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeZitadelCustomer wires the same routes fakeZitadelHandler does
// (handler_test.go), reused here so the customer path tests need no fixture
// duplication.
func fakeZitadelCustomer(t *testing.T, policyJSON, methodsJSON, factorsJSON string, finalized *atomic.Bool) *Client {
	t.Helper()
	return fakeZitadelHandler(t, policyJSON, methodsJSON, factorsJSON, finalized)
}

// TestCustomerLoginSetsNoCookie pins the property that distinguishes this
// endpoint from the merchant path: a successful login returns an identity
// but mints no session and sets no cookie. The storefront mints
// mp_customer_session itself, scoped to the exact request host.
func TestCustomerLoginSetsNoCookie(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelCustomer(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c)

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("cookies = %v, want none — the customer endpoint must never mint a session cookie", cookies)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		t.Fatalf("body = %v, want data.{uid,email}", body)
	}
	if data["uid"] != "u1" {
		t.Fatalf("data.uid = %v, want u1", data["uid"])
	}
	if data["email"] != "a@b.test" {
		t.Fatalf("data.email = %v, want a@b.test", data["email"])
	}
	if fin.Load() {
		t.Fatal("finalize was called — the customer path must decide and stop, never finalize (spec D11)")
	}
}

// TestCustomerLoginCollapsesUnknownUserAndWrongPasswordIntoOneAnswer mirrors
// TestLoginCollapsesUnknownUserAndWrongPasswordIntoOneAnswer for the merchant
// path (handler_test.go) — the enumeration concern is sharper here because
// this endpoint is reachable by anyone on a public storefront.
func TestCustomerLoginCollapsesUnknownUserAndWrongPasswordIntoOneAnswer(t *testing.T) {
	var responses []string
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
		h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
		rec := httptest.NewRecorder()
		h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
			strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x"}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "failedAttempts") || strings.Contains(rec.Body.String(), "not be found") {
			t.Fatalf("response leaks which half failed: %s", rec.Body.String())
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Fatalf("cookies set on a rejected login: %v", rec.Result().Cookies())
		}
		responses = append(responses, rec.Body.String())
	}
	if responses[0] != responses[1] {
		t.Fatalf("responses differ: %q vs %q — this is an account-enumeration oracle", responses[0], responses[1])
	}
}

// TestCustomerLoginFactorRequiredSetsNoCookie mirrors
// TestLoginFactorRequiredDoesNotMintSession: an outcome that still needs a
// TOTP code must not mint anything either.
func TestCustomerLoginFactorRequiredSetsNoCookie(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelCustomer(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c)

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("cookies = %v, want none for totp_required", rec.Result().Cookies())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["totp_required"] != true {
		t.Fatalf("body = %v, want totp_required: true", body)
	}
	if body["session_id"] != "s1" || body["session_token"] != "tok-1" {
		t.Fatalf("body = %v, want the session returned for the caller to collect a code", body)
	}
	if fin.Load() {
		t.Fatal("finalize was called for OutcomeFactorRequired")
	}
}

// TestCustomerLoginHandoffReturnsHostedLoginURL mirrors
// TestLoginHandoffReturnsHandoffURLAndDoesNotCallComplete.
func TestCustomerLoginHandoffReturnsHostedLoginURL(t *testing.T) {
	var fin atomic.Bool
	// Unrecognised factor type -> uncollectible -> OutcomeHandoff.
	c := fakeZitadelCustomer(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_U2F"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c).WithHostedLoginBaseURL("https://login.mark8ly.zitadel.cloud")

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, _ := body["handoff_url"].(string)
	if got != "https://login.mark8ly.zitadel.cloud/ui/v2/login/login" {
		t.Fatalf("handoff_url = %q", got)
	}
	if fin.Load() {
		t.Fatal("finalize was called for OutcomeHandoff")
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("cookies set on a handoff: %v", rec.Result().Cookies())
	}
}

// TestCustomerLoginHandoffWithNoBaseConfiguredFailsLoudlyNotSilently pins the
// fix for the gap where an unconfigured hosted login base URL produced a 200
// with an empty handoff_url: a customer with an uncollectible factor (a
// passkey, U2F, SMS OTP, recovery code, ...) and nowhere to go. That must
// never come back as a silent empty string — it must be a distinguishable
// error the storefront can render as "sign-in is unavailable".
func TestCustomerLoginHandoffWithNoBaseConfiguredFailsLoudlyNotSilently(t *testing.T) {
	var fin atomic.Bool
	// Unrecognised factor type -> uncollectible -> OutcomeHandoff.
	c := fakeZitadelCustomer(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_U2F"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c) // no WithHostedLoginBaseURL

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"login_name":"a@b.test","password":"x"}`)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["handoff_url"]; ok {
		t.Fatalf("body = %v, must not carry an empty handoff_url", body)
	}
	if body["error"] != "signin_unavailable" {
		t.Fatalf("body = %v, want error: signin_unavailable", body)
	}
	if fin.Load() {
		t.Fatal("finalize was called for OutcomeHandoff")
	}
}

// TestCustomerTotpWrongCodeReturns401WithoutLeakingZitadelBody mirrors
// TestTotpWrongCodeReturns401WithoutLeakingZitadelBody.
func TestCustomerTotpWrongCodeReturns401WithoutLeakingZitadelBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":3,"message":"Invalid code (COMMAND-Sfeg2)","details":[{"id":"COMMAND-Sfeg2","failedAttempts":2}]}`))
	}))
	defer srv.Close()
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))

	rec := httptest.NewRecorder()
	h.totp(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/totp",
		strings.NewReader(`{"auth_request_id":"V2_1","session_id":"s1","session_token":"tok-1","code":"000000"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "failedAttempts") {
		t.Fatalf("response leaks the attempt counter: %s", rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("cookies set on a rejected totp: %v", rec.Result().Cookies())
	}
}

// TestCustomerTotpCompleteReturnsIdentityAndSetsNoCookie exercises the full
// totp completion path and re-checks the no-cookie property there too.
func TestCustomerTotpCompleteReturnsIdentityAndSetsNoCookie(t *testing.T) {
	var fin atomic.Bool
	factorsWithTOTP := `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"},"totp":{"verifiedAt":"2026-09-03T01:01:00Z"}}}}`
	c := fakeZitadelCustomer(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsWithTOTP, &fin)
	h := NewCustomerHandler(c)

	rec := httptest.NewRecorder()
	h.totp(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/totp",
		strings.NewReader(`{"auth_request_id":"V2_1","session_id":"s1","session_token":"tok-1","code":"123456"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("cookies = %v, want none after totp completion", rec.Result().Cookies())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["uid"] != "u1" || data["email"] != "a@b.test" {
		t.Fatalf("body = %v, want data.{uid: u1, email: a@b.test}", body)
	}
	if fin.Load() {
		t.Fatal("finalize was called — the customer path must decide and stop, never finalize (spec D11)")
	}
}

// TestCustomerLoginResolvesEmailFromZitadelNotRequestBody mirrors
// TestTotpIgnoresAClientSuppliedLoginNameAndResolvesTheRealUser: the email in
// the response must come from Client.UserEmail, never from the submitted
// login_name — the same defect fixed on the merchant path in phase 2, and
// worse here since login_name is the only per-request text this endpoint
// takes from an anonymous caller.
func TestCustomerLoginResolvesEmailFromZitadelNotRequestBody(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelCustomer(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c)

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"attacker-chosen@evil.test","password":"x"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		t.Fatalf("body = %v, want data", body)
	}
	if data["email"] == "attacker-chosen@evil.test" {
		t.Fatalf("data.email = %v — the submitted login_name reached the response instead of the Zitadel-resolved email", data["email"])
	}
	if data["email"] != "a@b.test" {
		t.Fatalf("data.email = %v, want the email resolved from Zitadel (a@b.test)", data["email"])
	}
}

func TestCustomerLoginRejectsMissingFields(t *testing.T) {
	h := NewCustomerHandler(New("http://unused.invalid", "pat", nil))
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCustomerTotpRejectsMissingFields(t *testing.T) {
	h := NewCustomerHandler(New("http://unused.invalid", "pat", nil))
	rec := httptest.NewRecorder()
	h.totp(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/totp", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCustomerHandoffURLEmptyWithoutConfiguredBase(t *testing.T) {
	h := NewCustomerHandler(New("http://unused.invalid", "pat", nil))
	if got := h.handoffURL(); got != "" {
		t.Fatalf("handoffURL = %q, want empty when no base configured", got)
	}
}

// fakeZitadelCustomerIDP wires the same idp_intents/users/links/human routes
// fakeZitadelHandler wires, plus a customizable idp_intents response, so the
// customer idp/start and idp/finish tests can drive Google sign-in against
// one fake without duplicating handler_test.go's whole fixture.
func fakeZitadelCustomerIDP(t *testing.T, idpIntentBody string, extra map[string]http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		for pattern, fn := range extra {
			parts := strings.SplitN(pattern, " ", 2)
			if len(parts) == 2 && r.Method == parts[0] && strings.HasPrefix(r.URL.Path, parts[1]) {
				fn(w, r)
				return
			}
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/idp_intents":
			w.Write([]byte(`{"authUrl":"https://idp.example.test/auth?state=intent-1"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(idpIntentBody))
		default:
			t.Errorf("unexpected call: %s (no fixture wired for it)", key)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}

// TestCustomerIDPStartRejectsDisallowedReturnURL mirrors
// TestIDPStartRejectsDisallowedReturnURL: a host outside the configured
// (storefront) allowlist must be rejected before StartIDPIntent ever runs —
// proven the unreachableZitadel way, not merely by asserting the response.
func TestCustomerIDPStartRejectsDisallowedReturnURL(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewCustomerHandler(c).
		WithReturnURLAllowlist(mustAllowlist(t, nil, []string{"mark8ly.com"})).
		WithGoogleIDPID(testGoogleIDPID)

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/start",
		strings.NewReader(`{"return_url":"https://evil.example.com/x"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerIDPStartUsesTheConfiguredAllowlistNotAHardcodedOne pins that
// idp/start validates against WHATEVER ReturnURLAllowlist was configured on
// this CustomerHandler — main.go's job is wiring the STOREFRONT allowlist
// (ZitadelReturnURLAllowedHosts/SuffixesStorefront) into it, never the admin
// one, so a merchant-only host must be rejected when the configured
// allowlist does not carry it, exactly like any other disallowed host.
func TestCustomerIDPStartUsesTheConfiguredAllowlistNotAHardcodedOne(t *testing.T) {
	c := unreachableZitadel(t)
	// A storefront allowlist that does NOT include the admin host.
	h := NewCustomerHandler(c).
		WithReturnURLAllowlist(mustAllowlist(t, nil, []string{"shop.mark8ly.com"})).
		WithGoogleIDPID(testGoogleIDPID)

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/start",
		strings.NewReader(`{"return_url":"https://admin.mark8ly.com/auth/idp/finish"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s — the admin host is not in this handler's configured allowlist", rec.Code, rec.Body.String())
	}
}

func TestCustomerIDPStartReturnsAuthURLForAnAllowedStorefrontHost(t *testing.T) {
	c := fakeZitadelCustomerIDP(t, `{}`, nil)
	h := NewCustomerHandler(c).
		WithReturnURLAllowlist(mustAllowlist(t, nil, []string{"mark8ly.com"})).
		WithGoogleIDPID(testGoogleIDPID)

	rec := httptest.NewRecorder()
	h.idpStart(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/start",
		strings.NewReader(`{"return_url":"https://shop.mark8ly.com/auth/idp/finish"}`)))

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

// TestCustomerIDPFinishIgnoresATamperedUserBodyField mirrors
// TestIDPFinishIgnoresATamperedUserQueryParam: `user` is a JSON body field
// on this endpoint (there is no query param involved at all — the frontend
// forwards whatever it received in Zitadel's redirect as a body field), and
// it is attacker-controlled and must never be consulted, on the customer
// path any more than the merchant one. Every case here is a value genuinely
// DIFFERENT from the real resolved id ("u1") — a case equal to it would
// prove nothing, since the field being silently ignored and the field being
// (coincidentally) correct are indistinguishable.
func TestCustomerIDPFinishIgnoresATamperedUserBodyField(t *testing.T) {
	for _, tamperedUser := range []string{"", "some-other-victim-user-id", "u2"} {
		t.Run("user="+tamperedUser, func(t *testing.T) {
			idpIntentBody := `{"userId":"u1","idpInformation":{"idpId":"` + testGoogleIDPID + `","rawInformation":{"email":"a@b.test","email_verified":true}}}`
			extra := map[string]http.HandlerFunc{
				"GET /v2/users/": func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`{"user":{"human":{"email":{"email":"a@b.test"}}}}`))
				},
			}
			c := fakeZitadelCustomerIDP(t, idpIntentBody, extra)
			h := NewCustomerHandler(c).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")

			body := `{"intent_id":"i1","intent_token":"tok","user":"` + tamperedUser + `"}`
			rec := httptest.NewRecorder()
			h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish", strings.NewReader(body)))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var respBody map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
				t.Fatalf("decode: %v", err)
			}
			data, _ := respBody["data"].(map[string]any)
			if data == nil || data["uid"] != "u1" {
				t.Fatalf("body = %v, want data.uid = u1 regardless of the tampered user field", respBody)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatalf("cookies set: %v — the customer idp path must never mint a session", rec.Result().Cookies())
			}
		})
	}
}

func TestCustomerIDPFinishRejectsMissingFields(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewCustomerHandler(c)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestCustomerIDPFinishRejectsAnIntentFromAnUnexpectedIDP mirrors
// TestIDPFinishRejectsAnIntentFromAnUnexpectedIDP: an Apple (or any
// non-Google) intent must be refused before any lookup, link, or create.
func TestCustomerIDPFinishRejectsAnIntentFromAnUnexpectedIDP(t *testing.T) {
	idpIntentBody := `{"idpInformation":{"idpId":"some-other-idp","userId":"ext-1","userName":"victim@merchant.com","rawInformation":{"email":"victim@merchant.com","email_verified":true}}}`
	c := fakeZitadelCustomerIDP(t, idpIntentBody, nil)
	h := NewCustomerHandler(c).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")

	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerIDPFinishRefusesUnlinkedIdentityWithUnverifiedEmail mirrors the
// merchant test of the same shape: the gate is absolute, proven the
// unreachableZitadel way — no lookup, link, or create call may be reached.
func TestCustomerIDPFinishRefusesUnlinkedIdentityWithUnverifiedEmail(t *testing.T) {
	idpIntentBody := `{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"new.person@gmail.com","rawInformation":{"email":"new.person@gmail.com","email_verified":false}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(idpIntentBody))
		default:
			t.Errorf("must not be called for an unverified email: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerIDPFinishRefusesUnlinkedIdentityWithNoEmailClaim mirrors the
// merchant test: an absent email_verified claim must refuse identically to
// an explicit false.
func TestCustomerIDPFinishRefusesUnlinkedIdentityWithNoEmailClaim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"","rawInformation":{}}}`))
		default:
			t.Errorf("must not be called with no email claim: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerIDPFinishRefusesAnAmbiguousEmailMatchAndCreatesNothing mirrors
// the merchant test of the same shape.
func TestCustomerIDPFinishRefusesAnAmbiguousEmailMatchAndCreatesNothing(t *testing.T) {
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

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerIDPFinishLinksAFirstTimeIdentityToAnExistingVerifiedAccount is
// the customer-path analogue of the merchant link test: an unlinked but
// verified identity that matches an existing account is linked, not
// duplicated, and no session or finalize call ever happens.
func TestCustomerIDPFinishLinksAFirstTimeIdentityToAnExistingVerifiedAccount(t *testing.T) {
	var linkCalled, createCalled, sessionCalled atomic.Bool
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
			sessionCalled.Store(true)
			t.Error("the customer idp path must never create a Zitadel session")
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			w.Write([]byte(`{"user":{"human":{"email":{"email":"person@gmail.com"}}}}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !linkCalled.Load() {
		t.Fatal("LinkIDPToUser (POST /v2/users/existing-1/links) was never called")
	}
	if createCalled.Load() {
		t.Fatal("a new user must not be created when the existing account was linked")
	}
	if sessionCalled.Load() {
		t.Fatal("a Zitadel session must never be created on the customer idp path")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["uid"] != "existing-1" || data["email"] != "person@gmail.com" {
		t.Fatalf("body = %v, want data.{uid: existing-1, email: person@gmail.com}", body)
	}
}

// TestCustomerIDPFinishCreatesAUserWhenNoExistingAccountMatches is the
// customer-path registration case that has no merchant analogue at all: a
// verified, unlinked identity with no existing match must be CREATED here —
// the opposite of the merchant path's link-only refusal — because
// self-registration is the whole point of a storefront sign-in.
func TestCustomerIDPFinishCreatesAUserWhenNoExistingAccountMatches(t *testing.T) {
	var createCalled, linkCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"new.person@gmail.com","rawInformation":{"email":"new.person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			createCalled.Store(true)
			w.Write([]byte(`{"userId":"new-1"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/links"):
			linkCalled.Store(true)
			t.Error("must not link when there is no existing account to link to")
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			w.Write([]byte(`{"user":{"human":{"email":{"email":"new.person@gmail.com"}}}}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !createCalled.Load() {
		t.Fatal("CreateHumanUserWithIDPLink (POST /v2/users/human) was never called")
	}
	if linkCalled.Load() {
		t.Fatal("LinkIDPToUser must not be called when a new account was created instead")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["uid"] != "new-1" || data["email"] != "new.person@gmail.com" {
		t.Fatalf("body = %v, want data.{uid: new-1, email: new.person@gmail.com}", body)
	}
}

// TestCustomerIDPFinishRefusesAnEmailAlreadyTakenByAnUnverifiedAccountDistinctly
// is review Finding 1/4's fix: FindUserByVerifiedEmail found no VERIFIED
// match (so this path reaches CreateHumanUserWithIDPLink), but the create
// itself 400s because the email is already held by an unverified account.
// This is usually a PERMANENT lockout, not a race — refusing is correct
// either way (Google proving ownership does not make it safe to link to an
// account someone else may control), but it MUST answer a distinct code
// (email_taken) from the pre-create ambiguity case (email_ambiguous):
// collapsing them would make a permanent dead end look like a transient
// race the customer could retry past.
func TestCustomerIDPFinishRefusesAnEmailAlreadyTakenByAnUnverifiedAccountDistinctly(t *testing.T) {
	var linkCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			// No VERIFIED match — this is exactly why CreateHumanUserWithIDPLink
			// gets attempted below.
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":6,"message":"user already exists (COMMAND-oR9nS)","details":[{"id":"COMMAND-oR9nS"}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/links"):
			linkCalled.Store(true)
			t.Error("must not link to an account this endpoint never verified belongs to this identity")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("must not go further: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "email_taken" {
		t.Fatalf("body = %v, want error: email_taken — distinct from the pre-create email_ambiguous case", body)
	}
	if linkCalled.Load() {
		t.Fatal("must not attempt to link")
	}
}

// TestCustomerIDPFinishSurfacesAnInvalidIntentWithoutLeakingZitadelBody
// mirrors the merchant test.
func TestCustomerIDPFinishSurfacesAnInvalidIntentWithoutLeakingZitadelBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"intent not found (COMMAND-2Ls8f)","details":[{"id":"COMMAND-2Ls8f"}]}`))
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID)
	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish",
		strings.NewReader(`{"intent_id":"i1","intent_token":"tok"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "COMMAND-2Ls8f") {
		t.Fatalf("response leaks the Zitadel error id: %s", rec.Body.String())
	}
}

// TestCustomerIDPFinishSecondSignInResolvesViaUserIDAfterLinking mirrors the
// merchant test proving the point of linking: once linked, a subsequent
// retrieve resolves directly and never links or creates again.
func TestCustomerIDPFinishSecondSignInResolvesViaUserIDAfterLinking(t *testing.T) {
	var linked atomic.Bool
	linkCalls, createCalls := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			if linked.Load() {
				w.Write([]byte(`{"userId":"existing-1","idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
			} else {
				w.Write([]byte(`{"idpInformation":{"idpId":"` + testGoogleIDPID + `","userId":"ext-1","userName":"person@gmail.com","rawInformation":{"email":"person@gmail.com","email_verified":true}}}`))
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/existing-1/links":
			linkCalls++
			linked.Store(true)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			createCalls++
			w.Write([]byte(`{"userId":"should-not-happen"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			w.Write([]byte(`{"user":{"human":{"email":{"email":"person@gmail.com"}}}}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithGoogleIDPID(testGoogleIDPID).WithOrgID("org-1")
	body := `{"intent_id":"i1","intent_token":"tok"}`

	rec1 := httptest.NewRecorder()
	h.idpFinish(rec1, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish", strings.NewReader(body)))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first sign-in: status = %d, body = %s", rec1.Code, rec1.Body.String())
	}

	rec2 := httptest.NewRecorder()
	h.idpFinish(rec2, httptest.NewRequest(http.MethodPost, "/auth/customer/idp/finish", strings.NewReader(body)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second sign-in: status = %d, body = %s", rec2.Code, rec2.Body.String())
	}

	if linkCalls != 1 {
		t.Fatalf("LinkIDPToUser was called %d times, want exactly 1", linkCalls)
	}
	if createCalls != 0 {
		t.Fatalf("CreateHumanUserWithIDPLink was called %d times, want 0", createCalls)
	}
}

// TestCustomerEndpointsIgnoreAuthRequestID pins that a caller who still
// sends auth_request_id (e.g. a client not yet updated) gets it silently
// ignored rather than rejected — the field carries no meaning on this path
// and decodeJSON does not reject unknown fields.
func TestCustomerEndpointsIgnoreAuthRequestID(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelCustomer(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c)

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s — a stray auth_request_id must not cause a 400", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["auth_request_id"]; ok {
		t.Fatalf("body = %v, must not echo auth_request_id back", body)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["uid"] != "u1" {
		t.Fatalf("body = %v, want data.uid = u1", body)
	}
}

// recordingMailer is a CustomerVerificationMailer test double that counts
// calls and captures what it was asked to send, so register tests can
// assert "exactly one email, to this address, with this code" without a
// real notify.Client or platform-api.
type recordingMailer struct {
	calls int
	email string
	code  string
	ttl   time.Duration
	err   error
}

func (m *recordingMailer) SendLoginCode(ctx context.Context, email, code string, ttl time.Duration) error {
	m.calls++
	m.email, m.code, m.ttl = email, code, ttl
	return m.err
}

// captureLogs redirects the default slog logger to a buffer for the
// duration of the test and restores the previous logger on cleanup. Used to
// assert the verification code appears in NO log line — see
// TestCustomerRegisterNeverLeaksTheEmailCode.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// TestCustomerRegisterCreatesAndSendsExactlyOneEmail is the happy path: a
// brand-new email creates a Zitadel user with a password and returnCode,
// and the emailCode Zitadel hands back is delivered through exactly one
// call to the verification mailer — never returned in the response.
func TestCustomerRegisterCreatesAndSendsExactlyOneEmail(t *testing.T) {
	var createCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			// FindUserByVerifiedEmail: no existing account.
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			createCalled.Store(true)
			w.Write([]byte(`{"userId":"new-1","emailCode":"837291"}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	mailer := &recordingMailer{}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"new.shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !createCalled.Load() {
		t.Fatal("CreateHumanUserWithPassword (POST /v2/users/human) was never called")
	}
	if mailer.calls != 1 {
		t.Fatalf("mailer.calls = %d, want exactly 1", mailer.calls)
	}
	if mailer.email != "new.shopper@example.com" {
		t.Fatalf("mailer.email = %q", mailer.email)
	}
	if mailer.code != "837291" {
		t.Fatalf("mailer.code = %q, want the emailCode Zitadel returned", mailer.code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["uid"] != "new-1" || data["email"] != "new.shopper@example.com" {
		t.Fatalf("body = %v, want data.{uid: new-1, email: new.shopper@example.com}", body)
	}
	if strings.Contains(rec.Body.String(), "837291") {
		t.Fatalf("response body leaks the verification code: %s", rec.Body.String())
	}
}

// TestCustomerRegisterRefusesAnExistingVerifiedEmailAndCreatesNothing pins
// the pre-create refusal: FindUserByVerifiedEmail finding a match must stop
// this endpoint before it ever reaches CreateHumanUserWithPassword or the
// mailer.
func TestCustomerRegisterRefusesAnExistingVerifiedEmailAndCreatesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"shopper@example.com","isVerified":true}}}]}`))
		default:
			t.Errorf("must not create or send anything for an already-verified email: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	mailer := &recordingMailer{}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "email_taken" {
		t.Fatalf("body = %v, want error: email_taken", body)
	}
	if mailer.calls != 0 {
		t.Fatalf("mailer.calls = %d, want 0 — no account was created", mailer.calls)
	}
}

// TestCustomerRegisterSurfacesAWeakPasswordDistinctly pins that a
// too-short/weak password answers its own outcome, not a generic failure —
// verified live in phase 5, details[0].id == DOMAIN-HuJf6.
func TestCustomerRegisterSurfacesAWeakPasswordDistinctly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":3,"message":"Password is too short (DOMAIN-HuJf6)","details":[{"id":"DOMAIN-HuJf6"}]}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	mailer := &recordingMailer{}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"shopper@example.com","password":"x"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "weak_password" {
		t.Fatalf("body = %v, want error: weak_password — distinct from a generic failure", body)
	}
	if mailer.calls != 0 {
		t.Fatalf("mailer.calls = %d, want 0 — no account was created", mailer.calls)
	}
}

// TestCustomerRegisterRejectsAnUnauthenticatedCallerBeforeReachingZitadel
// pins the absolute constraint on this endpoint: a caller with no (or the
// wrong) internal secret must never cause a Zitadel call, exactly like
// login/totp/idp — see unreachableZitadel's doc.
func TestCustomerRegisterRejectsAnUnauthenticatedCallerBeforeReachingZitadel(t *testing.T) {
	c := unreachableZitadel(t)
	mailer := &recordingMailer{}
	h := NewCustomerHandler(c).WithInternalAuth(testInternalSecret).WithOrgID("org-1").WithNotify(mailer)

	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if mailer.calls != 0 {
		t.Fatalf("mailer.calls = %d, want 0 — an unauthenticated caller must never trigger an email send", mailer.calls)
	}
}

// TestCustomerRegisterNeverLeaksTheEmailCode is the single most important
// property of this endpoint: the emailCode Zitadel returns must appear in
// no response body reaching the browser, and in no log line, across every
// outcome this handler can reach it through.
func TestCustomerRegisterNeverLeaksTheEmailCode(t *testing.T) {
	const secretCode = "947261"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.Write([]byte(`{"userId":"new-1","emailCode":"` + secretCode + `"}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	logs := captureLogs(t)
	mailer := &recordingMailer{}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"new.shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if mailer.code != secretCode {
		t.Fatalf("mailer never received the code — test setup is broken")
	}
	if strings.Contains(rec.Body.String(), secretCode) {
		t.Fatalf("response body leaks the verification code: %s", rec.Body.String())
	}
	if strings.Contains(logs.String(), secretCode) {
		t.Fatalf("log output leaks the verification code: %s", logs.String())
	}
}

// TestCustomerRegisterRollsBackTheAccountWhenTheEmailFailsToSend is the fix
// for review Finding 1: a send failure must not strand an unverified,
// undeliverable account — the very next registration attempt for the same
// address would otherwise 400 forever (FindUserByVerifiedEmail finds no
// VERIFIED match, since the account can never be verified, so it always
// reaches CreateHumanUserWithPassword again, which always 400s on the
// duplicate email). register must delete the account it just created
// before answering, turning that into a clean retry instead.
func TestCustomerRegisterRollsBackTheAccountWhenTheEmailFailsToSend(t *testing.T) {
	var deleteCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.Write([]byte(`{"userId":"new-1","emailCode":"111222"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/users/new-1":
			deleteCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	mailer := &recordingMailer{err: errors.New("smtp unavailable")}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"new.shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body.String())
	}
	if !deleteCalled.Load() {
		t.Fatal("DeleteUser (DELETE /v2/users/new-1) was never called — the account is stranded, permanently burning this email")
	}
}

// TestCustomerRegisterRollsBackTheAccountWhenNoMailerConfigured mirrors the
// send-failure rollback for the other branch that can strand an account: no
// mailer configured at all.
func TestCustomerRegisterRollsBackTheAccountWhenNoMailerConfigured(t *testing.T) {
	var deleteCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.Write([]byte(`{"userId":"new-1","emailCode":"111222"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/users/new-1":
			deleteCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1")
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"new.shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body.String())
	}
	if !deleteCalled.Load() {
		t.Fatal("DeleteUser (DELETE /v2/users/new-1) was never called — the account is stranded with no mailer configured")
	}
}

// TestCustomerRegisterNeverLeaksTheEmailCodeWhenSendFails extends the leak
// property to the send-failure branch: a send error must not put the code
// into the response or into any log line, including whatever DeleteUser's
// rollback path logs.
func TestCustomerRegisterNeverLeaksTheEmailCodeWhenSendFails(t *testing.T) {
	const secretCode = "662551"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.Write([]byte(`{"userId":"new-1","emailCode":"` + secretCode + `"}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	logs := captureLogs(t)
	mailer := &recordingMailer{err: errors.New("smtp unavailable")}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"new.shopper@example.com","password":"test-password-not-real"}`)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if mailer.code != secretCode {
		t.Fatalf("mailer never received the code — test setup is broken")
	}
	if strings.Contains(rec.Body.String(), secretCode) {
		t.Fatalf("response body leaks the verification code: %s", rec.Body.String())
	}
	if strings.Contains(logs.String(), secretCode) {
		t.Fatalf("log output leaks the verification code: %s", logs.String())
	}
}

// TestCustomerRegisterNeverLeaksAnythingOnWeakPassword extends the leak
// property to the weak-password branch: CreateHumanUserWithPassword fails
// before Zitadel ever returns an emailCode here (there is nothing to leak
// on that front), so this pins the adjacent property the same test shape
// should cover — the submitted password itself must not appear in the
// response or in any log line either.
func TestCustomerRegisterNeverLeaksAnythingOnWeakPassword(t *testing.T) {
	const weakPassword = "x"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.Write([]byte(`{"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/human":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":3,"message":"Password is too short (DOMAIN-HuJf6)","details":[{"id":"DOMAIN-HuJf6"}]}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	logs := captureLogs(t)
	mailer := &recordingMailer{}
	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client())).WithOrgID("org-1").WithNotify(mailer)
	rec := httptest.NewRecorder()
	h.register(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/register",
		strings.NewReader(`{"email":"shopper@example.com","password":"`+weakPassword+`"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), weakPassword) {
		t.Fatalf("response body leaks the submitted password: %s", rec.Body.String())
	}
	if strings.Contains(logs.String(), weakPassword) {
		t.Fatalf("log output leaks the submitted password: %s", logs.String())
	}
	if mailer.calls != 0 {
		t.Fatalf("mailer.calls = %d, want 0 — no account was created", mailer.calls)
	}
}

// TestCustomerLoginRejectsAnUnverifiedAccount is the fix for review Finding
// 2: register (above) creates the first UNVERIFIED password accounts this
// system has ever had, and nothing gated sign-in on that flag. Without this,
// an attacker registers victim@example.com with a password of their own
// choosing and signs in normally — permanently burning that address for
// every future sign-up path, including Google, which is exactly the
// lockout email verification exists to prevent.
func TestCustomerLoginRejectsAnUnverifiedAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sessions":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"sessionId":"s1","sessionToken":"tok-1"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(factorsPasswordOnly))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/users/"):
			w.Write([]byte(`{"user":{"human":{"email":{"email":"victim@example.com","isVerified":false}}}}`))
		default:
			t.Errorf("must not reach sufficiency/finalize for an unverified account: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"login_name":"victim@example.com","password":"attacker-chosen-pw"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "email_not_verified" {
		t.Fatalf("body = %v, want error: email_not_verified", body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("cookies = %v, want none for an unverified account", rec.Result().Cookies())
	}
}

// TestCustomerLoginAcceptsAVerifiedAccount is the control for the test
// above: fakeZitadelCustomer's fixture answers isVerified:true, and every
// other login test in this file builds on it — this pins that the gate does
// not also block the ordinary case.
func TestCustomerLoginAcceptsAVerifiedAccount(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelCustomer(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewCustomerHandler(c)

	rec := httptest.NewRecorder()
	h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/login",
		strings.NewReader(`{"login_name":"a@b.test","password":"x"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerVerifyEmailAcceptsACorrectCode is the happy path: a correct
// code flips the account verified, and the response says so without
// echoing anything Zitadel doesn't itself vouch for.
func TestCustomerVerifyEmailAcceptsACorrectCode(t *testing.T) {
	var verifyCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/email/verify"):
			verifyCalled.Store(true)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
	rec := httptest.NewRecorder()
	h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
		strings.NewReader(`{"uid":"new-1","code":"837291"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !verifyCalled.Load() {
		t.Fatalf("Zitadel's email/verify was never called")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["verified"] != true {
		t.Fatalf("body = %v, want data.verified = true", body)
	}
}

// TestCustomerVerifyEmailRejectsAWrongCodeDistinctly pins the core mapping
// this endpoint exists for: Zitadel's details[0].id == "COMMAND-eis9R" for a
// wrong/expired code must produce a distinct, truthful outcome — never a
// generic failure, and never anything that could read as "your password was
// wrong". Keyed off the id, not message text: phase 5 found two different
// failures whose messages differed by one word.
func TestCustomerVerifyEmailRejectsAWrongCodeDistinctly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/email/verify"):
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":3,"message":"Code is invalid (COMMAND-eis9R)","details":[{"id":"COMMAND-eis9R"}]}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
	rec := httptest.NewRecorder()
	h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
		strings.NewReader(`{"uid":"new-1","code":"000000"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, _ := body["error"].(string)
	if got == "" || got == "invalid_credentials" || got == "internal_error" || got == "zitadel_unavailable" {
		t.Fatalf("error = %q, want a distinct, truthful wrong-code outcome — not a generic failure and not anything implying a bad password", got)
	}
	if got != "invalid_verification_code" {
		t.Fatalf("error = %q, want invalid_verification_code", got)
	}
}

// TestCustomerVerifyEmailUnrecognisedErrorIDFallsThroughToGenericUnavailable
// pins the other half of the classifier: an error id this package has never
// seen (e.g. Zitadel's "bogus user" case, observed as details[0].id ==
// "COMMAND-ieJ2e" — a DIFFERENT id than the wrong-code one, sharing no
// prefix) must never be mistaken for a wrong code. Refuse to guess.
func TestCustomerVerifyEmailUnrecognisedErrorIDFallsThroughToGenericUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/email/verify"):
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":5,"message":"User not found (COMMAND-ieJ2e)","details":[{"id":"COMMAND-ieJ2e"}]}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
	rec := httptest.NewRecorder()
	h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
		strings.NewReader(`{"uid":"bogus","code":"000000"}`)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "zitadel_unavailable" {
		t.Fatalf("body = %v, want error: zitadel_unavailable — an unrecognised id must not be read as a wrong code", body)
	}
}

// TestCustomerVerifyEmailRejectsAnUnauthenticatedCallerBeforeReachingZitadel
// pins the same absolute constraint every endpoint in this file carries: a
// caller with no (or the wrong) internal secret must never cause a Zitadel
// call — proven the unreachableZitadel way, not merely by asserting the
// response.
func TestCustomerVerifyEmailRejectsAnUnauthenticatedCallerBeforeReachingZitadel(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewCustomerHandler(c).WithInternalAuth(testInternalSecret)

	rec := httptest.NewRecorder()
	h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
		strings.NewReader(`{"uid":"new-1","code":"837291"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestCustomerVerifyEmailNeverLogsTheCode pins the credential-handling
// discipline this endpoint must follow: the verification code the shopper
// submits is a live credential, exactly like emailCode in register, and
// must appear in no log line — on success, on a wrong code, and on an
// unavailable Zitadel.
func TestCustomerVerifyEmailNeverLogsTheCode(t *testing.T) {
	const secretCode = "552013"

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		logs := captureLogs(t)
		h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
		rec := httptest.NewRecorder()
		h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
			strings.NewReader(`{"uid":"new-1","code":"`+secretCode+`"}`)))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(logs.String(), secretCode) {
			t.Fatalf("log output leaks the verification code: %s", logs.String())
		}
	})

	t.Run("wrong code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":3,"message":"Code is invalid (COMMAND-eis9R)","details":[{"id":"COMMAND-eis9R"}]}`))
		}))
		defer srv.Close()

		logs := captureLogs(t)
		h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
		rec := httptest.NewRecorder()
		h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
			strings.NewReader(`{"uid":"new-1","code":"`+secretCode+`"}`)))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(logs.String(), secretCode) {
			t.Fatalf("log output leaks the verification code: %s", logs.String())
		}
	})

	t.Run("zitadel unavailable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		logs := captureLogs(t)
		h := NewCustomerHandler(New(srv.URL, "pat", srv.Client()))
		rec := httptest.NewRecorder()
		h.verifyEmail(rec, httptest.NewRequest(http.MethodPost, "/auth/customer/verify-email",
			strings.NewReader(`{"uid":"new-1","code":"`+secretCode+`"}`)))

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(logs.String(), secretCode) {
			t.Fatalf("log output leaks the verification code: %s", logs.String())
		}
	})
}
