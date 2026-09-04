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

// idpCompleteBody is the well-formed request body every test below starts
// from, mutated field-by-field for the missing-field cases.
const idpCompleteBody = `{"auth_request_id":"V2_1","login_name":"a@b.test","session_id":"s1","session_token":"tok-1","workspace_tenant":"t1"}`

// TestIDPCompleteRejectsUnauthenticatedCallerBeforeAnyZitadelCall proves the
// internalauth guard runs first, exactly like login/totp/idpFinish: an
// unauthenticated caller must not be able to complete (or probe) a session
// by posting to this endpoint.
func TestIDPCompleteRejectsUnauthenticatedCallerBeforeAnyZitadelCall(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).WithInternalAuth(testInternalSecret)

	rec := httptest.NewRecorder()
	h.idpComplete(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(idpCompleteBody)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
}

// TestIDPCompleteRejectsEachMissingFieldWithoutCallingZitadel walks every
// required field and proves it is rejected before any Zitadel call — the
// same discipline login/totp/idpFinish already apply to their own inputs.
func TestIDPCompleteRejectsEachMissingFieldWithoutCallingZitadel(t *testing.T) {
	full := map[string]string{
		"auth_request_id":  "V2_1",
		"login_name":       "a@b.test",
		"session_id":       "s1",
		"session_token":    "tok-1",
		"workspace_tenant": "t1",
	}
	for omit := range full {
		t.Run("missing_"+omit, func(t *testing.T) {
			c := unreachableZitadel(t)
			h := NewHandler(c, nil).WithInternalAuth(testInternalSecret)

			fields := map[string]string{}
			for k, v := range full {
				if k != omit {
					fields[k] = v
				}
			}
			body, err := json.Marshal(fields)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(string(body)))
			r.Header.Set("X-Internal-Auth", testInternalSecret)
			rec := httptest.NewRecorder()
			h.idpComplete(rec, r)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("omitting %q: status = %d, want 400, body = %s", omit, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestIDPCompleteMintsSessionOnSuccess proves the happy path: given a
// session already established by idp/finish's tenant_required branch, this
// endpoint runs the sufficiency decision (federated=true, matching
// idpFinish) and the injected gauntlet, and answers with a callback_url —
// the same shape totp's success path returns.
func TestIDPCompleteMintsSessionOnSuccess(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	var gotUID, gotEmail, gotTenant string
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotUID, gotEmail, gotTenant = lc.UID, lc.Email, lc.TenantID
		return CompleteResult{}, nil
	}).WithInternalAuth(testInternalSecret)

	r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(idpCompleteBody))
	r.Header.Set("X-Internal-Auth", testInternalSecret)
	rec := httptest.NewRecorder()
	h.idpComplete(rec, r)

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
	if !fin.Load() {
		t.Fatal("finalize was not called")
	}
	if gotUID != "u1" || gotEmail != "a@b.test" || gotTenant != "t1" {
		t.Fatalf("complete called with uid=%q email=%q tenant=%q", gotUID, gotEmail, gotTenant)
	}
}

// TestIDPCompleteIsExemptFromForceMFALocalOnly proves this endpoint threads
// federated=true into CompleteIfSufficient, exactly like idpFinish's own
// tail call — every session reaching this endpoint originates from a Google
// IDP intent (see idpFinish's tenant_required branch), never from a
// password session, so forceMfaLocalOnly must not apply to it.
func TestIDPCompleteIsExemptFromForceMFALocalOnly(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFALocalOnly, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	completeCalled := false
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		return CompleteResult{}, nil
	}).WithInternalAuth(testInternalSecret)

	r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(idpCompleteBody))
	r.Header.Set("X-Internal-Auth", testInternalSecret)
	rec := httptest.NewRecorder()
	h.idpComplete(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !completeCalled {
		t.Fatal("complete() was not called — forceMfaLocalOnly must not apply to this endpoint")
	}
	if !fin.Load() {
		t.Fatal("finalize was not called — forceMfaLocalOnly must not apply to this endpoint")
	}
}

// TestIDPCompleteStepUpDoesNotCollapseIntoA500 proves the MFA-required
// outcome is a distinct, successful (200) response — never routed through
// the generic error branch — exactly as login/totp already behave. This is
// what keeps a step-up outcome from being indistinguishable from a gauntlet
// refusal at the transport layer.
func TestIDPCompleteStepUpDoesNotCollapseIntoA500(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)

	completeCalled := false
	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		completeCalled = true
		return CompleteResult{}, nil
	}).WithInternalAuth(testInternalSecret)

	r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(idpCompleteBody))
	r.Header.Set("X-Internal-Auth", testInternalSecret)
	rec := httptest.NewRecorder()
	h.idpComplete(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (step-up must not be a 500), body = %s", rec.Code, rec.Body.String())
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

// errNotAMemberFixture and errFGAUnreachableFixture stand in for
// autologin.ErrNotMember / autologin.ErrFGAUnreachable without importing
// that package: zitadellogin deliberately knows nothing about autologin
// (see CompleteFunc's doc comment), so it cannot type-switch on those
// errors, and never needs to — the two tests below prove each refusal is
// exercised as its own, independently-verified scenario reaching the
// generic gauntlet-failure branch, rather than being silently merged with
// or mistaken for the step-up path proven distinct above.
var (
	errNotAMemberFixture     = errors.New("fixture: user is not a member of the tenant")
	errFGAUnreachableFixture = errors.New("fixture: openfga is unreachable")
)

// TestIDPCompleteSurfacesNotAMemberDistinctlyFromStepUp proves a "not a
// member" refusal from the gauntlet reaches this endpoint's generic
// gauntlet-failure branch (500, no session minted) rather than being
// confused with a step-up outcome (200, totp_required/email_otp_required).
func TestIDPCompleteSurfacesNotAMemberDistinctlyFromStepUp(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		return CompleteResult{}, errNotAMemberFixture
	}).WithInternalAuth(testInternalSecret)

	r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(idpCompleteBody))
	r.Header.Set("X-Internal-Auth", testInternalSecret)
	rec := httptest.NewRecorder()
	h.idpComplete(rec, r)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "member") {
		t.Fatalf("response leaks the gauntlet's internal refusal reason: %s", body)
	}
	if body := rec.Body.String(); strings.Contains(body, "totp_required") || strings.Contains(body, "email_otp_required") {
		t.Fatalf("not-a-member must not be reported as a step-up outcome: %s", body)
	}
}

// TestIDPCompleteSurfacesFGAUnreachableDistinctlyFromStepUp mirrors the
// not-a-member case for the other gauntlet refusal this endpoint must carry
// through unmolested: an FGA outage.
func TestIDPCompleteSurfacesFGAUnreachableDistinctlyFromStepUp(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)

	h := NewHandler(c, func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		return CompleteResult{}, errFGAUnreachableFixture
	}).WithInternalAuth(testInternalSecret)

	r := httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/complete", strings.NewReader(idpCompleteBody))
	r.Header.Set("X-Internal-Auth", testInternalSecret)
	rec := httptest.NewRecorder()
	h.idpComplete(rec, r)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "openfga") || strings.Contains(body, "unreachable") {
		t.Fatalf("response leaks the gauntlet's internal refusal reason: %s", body)
	}
	if body := rec.Body.String(); strings.Contains(body, "totp_required") || strings.Contains(body, "email_otp_required") {
		t.Fatalf("fga-unreachable must not be reported as a step-up outcome: %s", body)
	}
}
