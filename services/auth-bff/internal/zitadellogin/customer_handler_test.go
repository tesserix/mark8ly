package zitadellogin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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

// TestCustomerIDPFinishIgnoresATamperedUserQueryParam mirrors
// TestIDPFinishIgnoresATamperedUserQueryParam: `user` is attacker-controlled
// and must never be consulted, on the customer path any more than the
// merchant one.
func TestCustomerIDPFinishIgnoresATamperedUserQueryParam(t *testing.T) {
	for _, tamperedUser := range []string{"", "some-other-victim-user-id", "u1"} {
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
