package zitadellogin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mark8ly/auth-bff/internal/internalauth"
)

const testInternalSecret = "s3cret-internal"

// unreachableZitadel returns a Client whose every call fails the test. The
// point of the header check is that an unauthenticated caller never causes a
// credential check at all: if CreatePasswordSession (POST /v2/sessions) or
// any other Zitadel call is reached, the endpoint still worked as an oracle
// no matter what status it eventually returned.
func unreachableZitadel(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Zitadel was called (%s %s) for a request that failed internal auth; "+
			"the credential check must not happen for an unauthenticated caller", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}

func postJSON(t *testing.T, path, body, secret string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if secret != "" {
		r.Header.Set(internalauth.Header, secret)
	}
	return r
}

// endpoint names one of the four guarded routes so every case below runs
// against all of them rather than only the two added by this branch: the
// merchant routes are as publicly reachable as the customer ones.
type endpoint struct {
	name string
	path string
	body string
	// call invokes the handler under test with the given secret configured.
	call func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request)
}

func guardedEndpoints() []endpoint {
	merchantComplete := func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	}
	return []endpoint{
		{
			name: "zitadel/login",
			path: "/auth/zitadel/login",
			body: `{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x","workspace_tenant":"t1"}`,
			call: func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request) {
				NewHandler(c, merchantComplete).WithInternalAuth(testInternalSecret).login(rec, r)
			},
		},
		{
			name: "zitadel/totp",
			path: "/auth/zitadel/totp",
			body: `{"auth_request_id":"V2_1","login_name":"a@b.test","session_id":"s1","session_token":"tok-1","code":"123456","workspace_tenant":"t1"}`,
			call: func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request) {
				NewHandler(c, merchantComplete).WithInternalAuth(testInternalSecret).totp(rec, r)
			},
		},
		{
			name: "zitadel/idp/start",
			path: "/auth/zitadel/idp/start",
			body: `{"return_url":"https://admin.mark8ly.com/auth/idp/finish"}`,
			call: func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request) {
				NewHandler(c, merchantComplete).WithInternalAuth(testInternalSecret).
					WithReturnURLAllowlist(mustAllowlist(t, []string{"admin.mark8ly.com"}, nil)).
					idpStart(rec, r)
			},
		},
		{
			name: "zitadel/idp/finish",
			path: "/auth/zitadel/idp/finish",
			body: `{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok-1","workspace_tenant":"t1"}`,
			call: func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request) {
				NewHandler(c, merchantComplete).WithInternalAuth(testInternalSecret).idpFinish(rec, r)
			},
		},
		{
			name: "customer/login",
			path: "/auth/customer/login",
			body: `{"login_name":"a@b.test","password":"x"}`,
			call: func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request) {
				NewCustomerHandler(c).WithInternalAuth(testInternalSecret).login(rec, r)
			},
		},
		{
			name: "customer/totp",
			path: "/auth/customer/totp",
			body: `{"session_id":"s1","session_token":"tok-1","code":"123456"}`,
			call: func(t *testing.T, c *Client, rec *httptest.ResponseRecorder, r *http.Request) {
				NewCustomerHandler(c).WithInternalAuth(testInternalSecret).totp(rec, r)
			},
		},
	}
}

// TestGuardedEndpointsRejectMissingAndWrongSecretIdentically is the core of
// the fix: all four credential endpoints answer the same 401 with the same
// body whether the header is absent or wrong, and neither case reaches
// Zitadel.
func TestGuardedEndpointsRejectMissingAndWrongSecretIdentically(t *testing.T) {
	for _, ep := range guardedEndpoints() {
		t.Run(ep.name, func(t *testing.T) {
			c := unreachableZitadel(t)

			missingRec := httptest.NewRecorder()
			ep.call(t, c, missingRec, postJSON(t, ep.path, ep.body, ""))

			wrongRec := httptest.NewRecorder()
			ep.call(t, c, wrongRec, postJSON(t, ep.path, ep.body, "not-the-secret"))

			if missingRec.Code != http.StatusUnauthorized {
				t.Errorf("missing header: status = %d, want 401 (body %s)", missingRec.Code, missingRec.Body.String())
			}
			if wrongRec.Code != http.StatusUnauthorized {
				t.Errorf("wrong header: status = %d, want 401 (body %s)", wrongRec.Code, wrongRec.Body.String())
			}
			if missingRec.Code != wrongRec.Code || missingRec.Body.String() != wrongRec.Body.String() {
				t.Errorf("missing (%d %s) and wrong (%d %s) headers must be indistinguishable",
					missingRec.Code, missingRec.Body.String(), wrongRec.Code, wrongRec.Body.String())
			}
			if got := strings.TrimSpace(missingRec.Body.String()); got != `{"error":"unauthorized"}` {
				t.Errorf("body = %s, want {\"error\":\"unauthorized\"}", got)
			}
		})
	}
}

// TestGuardedEndpointsRejectBeforeReadingTheBody pins that the guard runs
// ahead of request decoding too: a caller with no secret must not be able to
// tell a malformed body from a well-formed one.
func TestGuardedEndpointsRejectBeforeReadingTheBody(t *testing.T) {
	for _, ep := range guardedEndpoints() {
		t.Run(ep.name, func(t *testing.T) {
			c := unreachableZitadel(t)
			rec := httptest.NewRecorder()
			ep.call(t, c, rec, postJSON(t, ep.path, `not json at all`, ""))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 — auth must be decided before the body is parsed", rec.Code)
			}
		})
	}
}

// TestGuardedEndpointsAcceptTheCorrectSecret proves the guard does not
// simply close the routes: with the right header they reach Zitadel and
// behave exactly as before.
func TestGuardedEndpointsAcceptTheCorrectSecret(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA,
		`["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	for _, ep := range guardedEndpoints() {
		t.Run(ep.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ep.call(t, c, rec, postJSON(t, ep.path, ep.body, testInternalSecret))
			// The assertion is deliberately "not the guard's answer"
			// rather than a specific status: what each endpoint returns
			// past the guard depends on the Zitadel fixture (this one has
			// no hosted-login base URL, so a handoff outcome legitimately
			// answers 503) and is already pinned by handler_test.go and
			// customer_handler_test.go.
			if rec.Code == http.StatusUnauthorized {
				t.Fatalf("status = 401 with the correct header; body = %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `"unauthorized"`) {
				t.Fatalf("body = %s, want anything but the guard's rejection", rec.Body.String())
			}
		})
	}
}

// TestHandlersWithoutAConfiguredSecretStayOpen documents the deliberate
// shape of the check: an unset secret means "unchecked", because the guard
// that must never be bypassed lives at boot (config.ValidateZitadel), not
// here. Without this, every pre-existing handler test would have to be
// rewritten to carry a header.
func TestHandlersWithoutAConfiguredSecretStayOpen(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA,
		`["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	rec := httptest.NewRecorder()
	NewCustomerHandler(c).login(rec, postJSON(t, "/auth/customer/login", `{"login_name":"a@b.test","password":"x"}`, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

func TestInternalAuthorizedIsExactAndRejectsEmptyPresentedSecret(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"exact", testInternalSecret, true},
		{"empty", "", false},
		{"prefix", testInternalSecret[:5], false},
		{"suffix-extended", testInternalSecret + "x", false},
		{"case-changed", strings.ToUpper(testInternalSecret), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := postJSON(t, "/auth/customer/login", `{}`, tc.header)
			if got := internalAuthorized(r, testInternalSecret); got != tc.want {
				t.Fatalf("internalAuthorized = %v, want %v", got, tc.want)
			}
		})
	}
}
