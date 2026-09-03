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
