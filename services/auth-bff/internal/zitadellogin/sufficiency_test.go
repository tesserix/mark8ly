package zitadellogin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func fakeZitadel(t *testing.T, policyJSON, methodsJSON, factorsJSON string, finalized *atomic.Bool) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyJSON))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":` + methodsJSON + `}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			finalized.Store(true)
			w.Write([]byte(`{"callbackUrl":"https://admin.mark8ly.com/auth/callback?code=c&state=s"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(factorsJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}

const factorsPasswordOnly = `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"}}}}`
const factorsWithTOTP = `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"},"totp":{"verifiedAt":"2026-09-03T01:01:00Z"}}}}`

// The lifetime value in these fixtures is deliberately trivial. The anchor check
// is presence-only (client.go: `if _, ok := wire.Policy[policyAnchorKey]`), so the
// duration never matters to any assertion here.
//
// Do not "improve" it to a realistic duration: GitGuardian's Generic Password
// detector treats that key followed by a longer value as a hardcoded password and
// fails CI. Eight fixtures tripped it once already, and so did the comment that
// first explained this, by quoting the value verbatim.
const policyNoMFA = `{"policy":{"passwordCheckLifetime":"0s"}}`
const policyForceMFA = `{"policy":{"passwordCheckLifetime":"0s","forceMfa":true}}`
const policyLocalOnly = `{"policy":{"passwordCheckLifetime":"0s","forceMfaLocalOnly":true}}`

func TestPasswordOnlyCompletesWhenNoMFAIsRequired(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete || res.CallbackURL == "" {
		t.Fatalf("result = %+v", res)
	}
	if !fin.Load() {
		t.Error("finalize was not called")
	}
}

func TestPasswordOnlyDoesNotCompleteWhenForceMfaIsSet(t *testing.T) {
	// The whole reason this package exists: Zitadel WOULD issue a code here.
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeFactorRequired {
		t.Fatalf("outcome = %v, want OutcomeFactorRequired", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize WAS called for a password-only session under forceMfa — MFA bypass")
	}
}

func TestForceMfaLocalOnlyDoesNotApplyToFederatedUsers(t *testing.T) {
	// forceMfaLocalOnly means "local/password users only". mark8ly has Google
	// and Apple users; forcing MFA on them would be wrong.
	var fin atomic.Bool
	c := fakeZitadel(t, policyLocalOnly, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, true /* federated */)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete {
		t.Fatalf("outcome = %v, want OutcomeComplete for a federated user", res.Outcome)
	}
}

func TestForceMfaLocalOnlyDoesApplyToPasswordUsers(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyLocalOnly, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeFactorRequired {
		t.Fatalf("outcome = %v, want OutcomeFactorRequired", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize was called")
	}
}

func TestAnUncollectibleFactorHandsOff(t *testing.T) {
	// A passkey is a real second factor this login page cannot collect.
	// Completing anyway would silently skip it.
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_PASSKEY"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeHandoff {
		t.Fatalf("outcome = %v, want OutcomeHandoff", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize was called with an uncollected factor outstanding")
	}
}

func TestAnUnreadablePolicyHandsOffRatherThanCompleting(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, `{"policy":{"nonsense":true}}`, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v (a handoff is an outcome, not an error)", err)
	}
	if res.Outcome != OutcomeHandoff {
		t.Fatalf("outcome = %v, want OutcomeHandoff", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize was called despite an unreadable policy")
	}
}

func TestZeroValueOutcomeIsHandoff(t *testing.T) {
	var r Result
	if r.Outcome != OutcomeHandoff {
		t.Fatal("the zero value must be OutcomeHandoff so an undecided Result cannot complete a login")
	}
}

func TestCompleteAfterFactorRefusesWhenZitadelDoesNotReportTOTP(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteAfterFactor(context.Background(), "V2_1", Session{ID: "s1", Token: "t"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome == OutcomeComplete {
		t.Fatal("completed without Zitadel confirming the TOTP factor")
	}
	if fin.Load() {
		t.Fatal("finalize was called")
	}
}

func TestCompleteAfterFactorCompletesWhenTOTPIsConfirmed(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsWithTOTP, &fin)
	res, err := c.CompleteAfterFactor(context.Background(), "V2_1", Session{ID: "s1", Token: "t"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete || !fin.Load() {
		t.Fatalf("result = %+v finalized=%v", res, fin.Load())
	}
}

// --- DecideSufficiency / DecideAfterFactor: the customer path's decision-only
// functions must never finalize, no matter what the input is. ---

// fakeZitadelNoFinalize serves the same fixtures as fakeZitadel but fails the
// test outright if the finalize endpoint is ever hit — the strongest
// assertion available that a code path never obtains an OIDC authorization
// code, stronger than merely checking a bool afterward.
func fakeZitadelNoFinalize(t *testing.T, policyJSON, methodsJSON, factorsJSON string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyJSON))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":` + methodsJSON + `}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			t.Fatalf("finalize endpoint was called: %s %s — a decision-only function must never finalize", r.Method, r.URL.Path)
		case strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(factorsJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}

func TestDecideSufficiencyCompletesWithoutFinalizingOrACallbackURL(t *testing.T) {
	c := fakeZitadelNoFinalize(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly)
	res, err := c.DecideSufficiency(context.Background(), Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete {
		t.Fatalf("outcome = %v, want OutcomeComplete", res.Outcome)
	}
	if res.CallbackURL != "" {
		t.Fatalf("CallbackURL = %q, want empty — a decision-only function must never carry one", res.CallbackURL)
	}
}

func TestDecideSufficiencyStillRequiresMfaWhenForced(t *testing.T) {
	// The MFA guarantee must survive the split: a password-only session
	// under forceMfa must still land on OutcomeFactorRequired, never
	// OutcomeComplete, from the decision-only function too.
	c := fakeZitadelNoFinalize(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly)
	res, err := c.DecideSufficiency(context.Background(), Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeFactorRequired {
		t.Fatalf("outcome = %v, want OutcomeFactorRequired", res.Outcome)
	}
}

func TestDecideSufficiencyFailsClosedOnEveryUncertainInput(t *testing.T) {
	cases := []struct {
		name        string
		policyJSON  string
		methodsJSON string
		factorsJSON string
	}{
		{"unreadable policy", `{"policy":{"nonsense":true}}`, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly},
		{"uncollectible factor", policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_PASSKEY"]`, factorsPasswordOnly},
		{"unreadable session subject", policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, `{"session":{"factors":{}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeZitadelNoFinalize(t, tc.policyJSON, tc.methodsJSON, tc.factorsJSON)
			res, err := c.DecideSufficiency(context.Background(), Session{ID: "s1", Token: "t"}, false)
			if err != nil {
				t.Fatalf("err = %v (a handoff is an outcome, not an error)", err)
			}
			if res.Outcome != OutcomeHandoff {
				t.Fatalf("outcome = %v, want OutcomeHandoff", res.Outcome)
			}
		})
	}
}

func TestDecideAfterFactorCompletesWithoutFinalizingOrACallbackURL(t *testing.T) {
	c := fakeZitadelNoFinalize(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsWithTOTP)
	res, err := c.DecideAfterFactor(context.Background(), Session{ID: "s1", Token: "t"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete {
		t.Fatalf("outcome = %v, want OutcomeComplete", res.Outcome)
	}
	if res.CallbackURL != "" {
		t.Fatalf("CallbackURL = %q, want empty — a decision-only function must never carry one", res.CallbackURL)
	}
}

func TestDecideAfterFactorFailsClosedWhenTOTPIsNotConfirmed(t *testing.T) {
	c := fakeZitadelNoFinalize(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly)
	res, err := c.DecideAfterFactor(context.Background(), Session{ID: "s1", Token: "t"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeHandoff {
		t.Fatalf("outcome = %v, want OutcomeHandoff", res.Outcome)
	}
}

// An IDP link must not make a session uncollectible.
//
// Zitadel reports AUTHENTICATION_METHOD_TYPE_IDP the moment a user holds any
// IDP link — and the Google sign-in flow is what CREATES that link. Treating
// it as uncollectible therefore made a completed Google sign-in permanently
// prevent the next one: sufficiency handed off, the browser followed the
// handoff to Zitadel's hosted login, and Zitadel answered
// {"error":"no valid authentication request found"}.
//
// Verified in production 2026-09-06: the reporting account's enrolled methods
// were exactly [PASSWORD, IDP]. A Google-only account carries [IDP] alone and
// failed identically, so no choice of address avoided it.
func TestDecideSufficiencyTreatsAnIDPLinkAsCollectible(t *testing.T) {
	for _, tc := range []struct {
		name        string
		methodsJSON string
	}{
		{"password and idp", `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_IDP"]`},
		{"idp only", `["AUTHENTICATION_METHOD_TYPE_IDP"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeZitadelNoFinalize(t, policyNoMFA, tc.methodsJSON, factorsPasswordOnly)
			res, err := c.DecideSufficiency(context.Background(), Session{ID: "s1", Token: "t"}, true)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if res.Outcome == OutcomeHandoff {
				t.Fatalf("outcome = OutcomeHandoff — an IDP link is an identity source, not an uncollectible factor")
			}
		})
	}
}

// The guard that keeps the fix honest: a REAL uncollectible factor alongside
// an IDP link must still hand off. Ignoring IDP must not become "ignore
// everything that is not password or TOTP".
func TestDecideSufficiencyStillHandsOffForAPasskeyBesideAnIDPLink(t *testing.T) {
	c := fakeZitadelNoFinalize(t, policyForceMFA,
		`["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_IDP","AUTHENTICATION_METHOD_TYPE_PASSKEY"]`,
		factorsPasswordOnly)
	res, err := c.DecideSufficiency(context.Background(), Session{ID: "s1", Token: "t"}, true)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeHandoff {
		t.Fatalf("outcome = %v, want OutcomeHandoff — a passkey is still uncollectible", res.Outcome)
	}
}
