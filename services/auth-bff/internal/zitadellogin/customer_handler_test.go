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
