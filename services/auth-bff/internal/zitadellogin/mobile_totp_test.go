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

// factorsWithTOTPVerified is what Zitadel reports once the code has been
// submitted: DecideAfterFactor re-reads the session rather than trusting
// that VerifyTOTP returned without error, so the fixture must show it.
const factorsWithTOTPVerified = `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"},"totp":{"verifiedAt":"2026-09-03T01:01:00Z"}}}}`

func totpVerifyHandler(t *testing.T, ps *fakePendingStore, complete CompleteFunc) *Handler {
	t.Helper()
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFA,
		`["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`,
		factorsWithTOTPVerified, &fin)
	zit := zitadelTokenAndAuthorize(t)
	if complete == nil {
		complete = func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
			return CompleteResult{}, nil
		}
	}
	return NewHandler(c, complete).
		WithTokenIssuer(NewTokenExchanger(zit.URL, testClientID, testClientPlaceholder, zit.Client()),
			"https://admin.mark8ly.com/auth/callback", "proj-1").
		WithStepUp(&fakeCodeVerifier{}, ps)
}

func sealedTOTPPending() *fakePendingStore {
	return &fakePendingStore{opened: &PendingLogin{
		UID: "u1", Email: "a@b.test", TenantID: "t1",
		ZitadelSessionID: "s1", ZitadelSessionToken: "tok-1",
	}}
}

func postTOTPVerify(h *Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.mobileTOTPVerify(rec, httptest.NewRequest(http.MethodPost,
		"/auth/zitadel/mobile/totp/verify", strings.NewReader(body)))
	return rec
}

// The happy path: a correct code resumes the login the sealed token
// describes and yields tokens. This is the whole point of #686 item 2 — a
// merchant with TOTP enrolled could not sign in on mobile at all.
func TestMobileTOTPVerify_CorrectCodeIssuesTokens(t *testing.T) {
	h := totpVerifyHandler(t, sealedTOTPPending(), nil)

	rec := postTOTPVerify(h, `{"pending_token":"sealed","code":"123456"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body["data"].(map[string]any)
	if data["access_token"] != "AT" {
		t.Fatalf("want tokens, got %v", body)
	}
	// The tenant travels in the sealed token, because the app cannot know
	// it: a Zitadel token carries no tenant claim and discovery itself
	// needs one.
	if data["tenant_id"] != "t1" {
		t.Fatalf("tenant_id = %v, want the tenant from the sealed token", data["tenant_id"])
	}
}

// A wrong code is a TOTP rejection, and must be reported as one. Answering
// with the password path's code would have the app show "check your
// details" on a screen with no password field on it.
func TestMobileTOTPVerify_WrongCodeIsATOTPRejection(t *testing.T) {
	// A Zitadel that refuses the TOTP check. Only the PATCH matters here;
	// nothing past it should run.
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyForceMFA,
		`["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`,
		factorsWithTOTPVerified, &fin)
	c = rejectTOTPClient(t, c)
	zit := zitadelTokenAndAuthorize(t)
	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		t.Fatal("the gauntlet must not run for a rejected code")
		return CompleteResult{}, nil
	}).WithTokenIssuer(NewTokenExchanger(zit.URL, testClientID, testClientPlaceholder, zit.Client()),
		"https://admin.mark8ly.com/auth/callback", "proj-1").
		WithStepUp(&fakeCodeVerifier{}, sealedTOTPPending())

	rec := postTOTPVerify(h, `{"pending_token":"sealed","code":"000000"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_totp") {
		t.Fatalf("want the totp-specific error, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Fatal("a rejected code must not yield tokens")
	}
	if fin.Load() {
		t.Fatal("finalize must not run for a rejected code")
	}
}

// An expired or forged pending token is refused before Zitadel is touched:
// there is no session to submit the code against.
func TestMobileTOTPVerify_BadPendingTokenRefusedBeforeVerify(t *testing.T) {
	h := totpVerifyHandler(t, &fakePendingStore{err: errors.New("invalid session")}, nil)

	rec := postTOTPVerify(h, `{"pending_token":"forged","code":"123456"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_challenge") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

// A pending token with no Zitadel session handle cannot be resumed. That
// is our inconsistency, not the merchant's, and must not read as a bad
// code — otherwise a correct one is retyped forever.
func TestMobileTOTPVerify_MissingSessionHandleIsNotAuthFailure(t *testing.T) {
	h := totpVerifyHandler(t, &fakePendingStore{
		opened: &PendingLogin{UID: "u1", Email: "a@b.test", TenantID: "t1"},
	}, nil)

	rec := postTOTPVerify(h, `{"pending_token":"sealed","code":"123456"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (not 401)", rec.Code)
	}
}

// The route is internal-only, exactly like its siblings: it completes a
// login, so an unauthenticated caller must be refused before anything
// else happens.
func TestMobileTOTPVerify_RequiresInternalAuth(t *testing.T) {
	ps := sealedTOTPPending()
	h := totpVerifyHandler(t, ps, nil).WithInternalAuth("s3cret")

	rec := httptest.NewRecorder()
	h.mobileTOTPVerify(rec, httptest.NewRequest(http.MethodPost,
		"/auth/zitadel/mobile/totp/verify",
		strings.NewReader(`{"pending_token":"sealed","code":"123456"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Fatal("an unauthenticated caller must not obtain tokens")
	}
}

// A login needing BOTH step-ups chains: passing TOTP runs the gauntlet,
// which then demands the emailed code and hands back a fresh pending
// token. Answering with tokens here would skip the second gate entirely.
func TestMobileTOTPVerify_StillOutstandingEmailOTPChains(t *testing.T) {
	ps := sealedTOTPPending()
	ps.sealed = "second-sealed-value"
	h := totpVerifyHandler(t, ps, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	})

	rec := postTOTPVerify(h, `{"pending_token":"sealed","code":"123456"}`)

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	data, _ := body["data"].(map[string]any)
	if data["email_otp_required"] != true {
		t.Fatalf("want email_otp_required, got %v", body)
	}
	if data["pending_token"] != "second-sealed-value" {
		t.Fatalf("want a fresh pending_token for the second gate, got %v", data["pending_token"])
	}
	if _, leaked := data["access_token"]; leaked {
		t.Fatal("no token may be issued while a step-up is still outstanding")
	}
}

// rejectTOTPClient wraps the shared fixture with a server whose session
// PATCH — the TOTP submission — fails. Everything else 404s, because
// nothing else should be reached.
func rejectTOTPClient(t *testing.T, _ *Client) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/v2/sessions/") {
			// 400 is what Zitadel answers a wrong TOTP code with, and what
			// the client maps to ErrBadCredentials (see VerifyTOTP's
			// badRequestErr). A 401 here would be "Zitadel unavailable".
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"invalid code"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}
