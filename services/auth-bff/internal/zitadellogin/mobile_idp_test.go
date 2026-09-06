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

// mobileIDPFinishBody is the well-formed request the mobile surface posts
// after the bridge page hands the app back Zitadel's id/token pair. It
// carries an auth_request_id so the common cases below do not also depend
// on the mint path — that path has its own test.
const mobileIDPFinishBody = `{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"it1","workspace_tenant":"t1","provider":"google"}`

func mobileIDPHandler(t *testing.T, complete CompleteFunc) *Handler {
	t.Helper()
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	return NewHandler(c, complete).
		WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1").
		WithGoogleIDPID(testGoogleIDPID).
		WithOrgID("org-1")
}

func postMobileIDP(t *testing.T, h func(http.ResponseWriter, *http.Request), path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// The whole point of the mobile surface: a completed Google sign-in has to
// answer with a bearer token, because marketplace-api verifies a JWT and a
// native client can use neither a session cookie nor a callback_url.
func TestMobileIDPFinish_CompleteReturnsTokensNotCallbackURL(t *testing.T) {
	var gotUID, gotEmail, gotTenant string
	h := mobileIDPHandler(t, func(_ context.Context, _ http.ResponseWriter, lc LoginContext) (CompleteResult, error) {
		gotUID, gotEmail, gotTenant = lc.UID, lc.Email, lc.TenantID
		return CompleteResult{}, nil
	})

	rec, body := postMobileIDP(t, h.mobileIDPFinish, "/auth/zitadel/mobile/idp/finish", mobileIDPFinishBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data, _ := body["data"].(map[string]any)
	if data["access_token"] != "AT" || data["refresh_token"] != "RT" {
		t.Fatalf("want tokens, got %v", body)
	}
	// A live authorization code has no use on a device and must not leak.
	if _, leaked := body["callback_url"]; leaked {
		t.Fatalf("callback_url must not be returned to a mobile client: %v", body)
	}
	// The identity must come from Zitadel's own record, never the body.
	if gotUID == "" || gotEmail != "a@b.test" || gotTenant != "t1" {
		t.Fatalf("gauntlet did not run with the Zitadel-resolved identity: uid=%q email=%q tenant=%q", gotUID, gotEmail, gotTenant)
	}
}

// A fresh install is ALWAYS an unrecognised device, so a Google sign-in on
// mobile normally stops here. It must hand back a sealed pending_token —
// the app has no cookie to resume from — and NO tokens, or the step-up is
// decorative.
func TestMobileIDPFinish_StepUpSealsAPendingTokenAndIssuesNoTokens(t *testing.T) {
	h := mobileIDPHandler(t, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	}).WithStepUp(&fakeCodeVerifier{}, &fakePendingStore{sealed: "sealed-value"})

	rec, body := postMobileIDP(t, h.mobileIDPFinish, "/auth/zitadel/mobile/idp/finish", mobileIDPFinishBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data, _ := body["data"].(map[string]any)
	if data["email_otp_required"] != true {
		t.Fatalf("want email_otp_required, got %v", body)
	}
	if data["pending_token"] != "sealed-value" {
		t.Fatalf("want the sealed pending token, got %v", body)
	}
	if _, ok := data["access_token"]; ok {
		t.Fatal("tokens must NOT be issued while a step-up is outstanding")
	}
}

// The pin is provider-SELECTED. An intent that came from some other IDP on
// the same instance must be refused even though that IDP is perfectly
// valid to Zitadel — its email_verified claim is not Google's.
func TestMobileIDPFinish_RejectsAnIntentFromAnotherIDP(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	// Configured Google id deliberately differs from the id the fake
	// intent carries (testGoogleIDPID).
	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		t.Fatal("the gauntlet must not run for an intent from an unpinned IDP")
		return CompleteResult{}, nil
	}).WithTokenIssuer(tokenServer(t), "https://admin.mark8ly.com/auth/callback", "proj-1").
		WithGoogleIDPID("some-other-idp").
		WithOrgID("org-1")

	rec, body := postMobileIDP(t, h.mobileIDPFinish, "/auth/zitadel/mobile/idp/finish", mobileIDPFinishBody)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if body["error"] != "unexpected_idp" {
		t.Fatalf("error = %v, want unexpected_idp", body["error"])
	}
}

// Google and Apple are the only providers this handler trusts. Naming any
// other one is refused BEFORE Zitadel is called at all — adding a provider
// must be a deliberate switch case, never something a request can opt into.
func TestMobileIDPFinish_RejectsAnUnsupportedProviderWithoutCallingZitadel(t *testing.T) {
	for _, provider := range []string{"github", "facebook", "anything"} {
		t.Run(provider, func(t *testing.T) {
			c := unreachableZitadel(t)
			h := NewHandler(c, nil).WithGoogleIDPID(testGoogleIDPID).WithAppleIDPID(testAppleIDPID)

			rec, body := postMobileIDP(t, h.mobileIDPFinish, "/auth/zitadel/mobile/idp/finish",
				`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"it1","workspace_tenant":"t1","provider":"`+provider+`"}`)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
			if body["error"] != "unsupported_provider" {
				t.Fatalf("error = %v, want unsupported_provider", body["error"])
			}
		})
	}
}

// idp/start must refuse the same unknown provider, and must do so without
// starting an intent — otherwise the refusal above is the only thing
// stopping an intent being opened against an untrusted IDP.
func TestMobileIDPStart_RejectsAnUnsupportedProvider(t *testing.T) {
	allow, err := NewReturnURLAllowlist([]string{"admin.mark8ly.com"}, nil)
	if err != nil {
		t.Fatalf("allowlist: %v", err)
	}
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).WithGoogleIDPID(testGoogleIDPID).WithAppleIDPID(testAppleIDPID).WithReturnURLAllowlist(allow)

	rec, body := postMobileIDP(t, h.idpStart, "/auth/zitadel/mobile/idp/start",
		`{"return_url":"https://admin.mark8ly.com/auth/idp/mobile","provider":"github"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if body["error"] != "unsupported_provider" {
		t.Fatalf("error = %v, want unsupported_provider", body["error"])
	}
}

// The mobile bridge page's URL is still just a return URL, and it goes
// through the SAME allowlist every other IDP start does. Zitadel validates
// successUrl not at all, so a host that is not on the list must be refused
// here or nowhere.
func TestMobileIDPStart_StillEnforcesTheReturnURLAllowlist(t *testing.T) {
	allow, err := NewReturnURLAllowlist([]string{"admin.mark8ly.com"}, nil)
	if err != nil {
		t.Fatalf("allowlist: %v", err)
	}
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewHandler(c, nil).WithGoogleIDPID(testGoogleIDPID).WithReturnURLAllowlist(allow)

	cases := []struct {
		name, returnURL string
		want            int
	}{
		{"allowlisted host", "https://admin.mark8ly.com/auth/idp/mobile", http.StatusOK},
		{"another host entirely", "https://evil.example.com/auth/idp/mobile", http.StatusBadRequest},
		{"a merchant storefront subdomain", "https://evil-shop.mark8ly.com/auth/idp/mobile", http.StatusBadRequest},
		{"plain http", "http://admin.mark8ly.com/auth/idp/mobile", http.StatusBadRequest},
		{"the app's own custom scheme", "mark8ly-admin://auth/idp", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"return_url": tc.returnURL, "provider": "google"})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			rec, _ := postMobileIDP(t, h.idpStart, "/auth/zitadel/mobile/idp/start", string(body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// A native client has no browser round trip through /oauth/v2/authorize,
// so it cannot obtain an auth_request_id and the server mints one — the
// same accommodation loginMode already makes. Without this the mobile
// finish would 400 on every real request.
func TestMobileIDPFinish_MintsAnAuthRequestWhenTheCallerHasNone(t *testing.T) {
	zit := zitadelTokenAndAuthorize(t)
	var fin atomic.Bool
	c := fakeZitadelHandler(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	h := NewHandler(c, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	}).WithTokenIssuer(NewTokenExchanger(zit.URL, testClientID, testClientPlaceholder, zit.Client()),
		"https://admin.mark8ly.com/auth/callback", "proj-1").
		WithGoogleIDPID(testGoogleIDPID).
		WithOrgID("org-1")

	rec, body := postMobileIDP(t, h.mobileIDPFinish, "/auth/zitadel/mobile/idp/finish",
		`{"intent_id":"i1","intent_token":"it1","workspace_tenant":"t1","provider":"google"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (an absent auth_request_id must be minted, not rejected), body = %s", rec.Code, rec.Body.String())
	}
	data, _ := body["data"].(map[string]any)
	if data["access_token"] != "AT" {
		t.Fatalf("want tokens after a minted auth request, got %v", body)
	}
}

// The web finish must keep REQUIRING an auth_request_id: the browser
// already holds one, and minting a second would orphan the flow the
// merchant is actually in.
func TestIDPFinish_WebStillRequiresAnAuthRequestID(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).WithGoogleIDPID(testGoogleIDPID)

	rec, _ := postMobileIDP(t, h.idpFinish, "/auth/zitadel/idp/finish",
		`{"intent_id":"i1","intent_token":"it1","workspace_tenant":"t1"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// The tenant is unknowable until the identity is resolved, so the mobile
// finish answers tenant_required exactly like the web one — the app's
// backend then resolves the tenant by email and calls complete.
func TestMobileIDPFinish_WithoutATenantAsksForOne(t *testing.T) {
	h := mobileIDPHandler(t, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		t.Fatal("the gauntlet must not run before a tenant is chosen")
		return CompleteResult{}, nil
	})

	rec, body := postMobileIDP(t, h.mobileIDPFinish, "/auth/zitadel/mobile/idp/finish",
		`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"it1","provider":"google"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body["tenant_required"] != true || body["login_name"] != "a@b.test" {
		t.Fatalf("want tenant_required with the resolved login_name, got %v", body)
	}
	if body["session_id"] == "" || body["session_token"] == "" {
		t.Fatalf("tenant_required must carry the session to complete from, got %v", body)
	}
}

// The second leg: given the session finish handed back and the tenant its
// caller resolved, complete must answer with tokens.
func TestMobileIDPComplete_ReturnsTokens(t *testing.T) {
	h := mobileIDPHandler(t, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{}, nil
	})

	rec, body := postMobileIDP(t, h.mobileIDPComplete, "/auth/zitadel/mobile/idp/complete",
		`{"auth_request_id":"V2_1","login_name":"a@b.test","session_id":"s1","session_token":"tok-1","workspace_tenant":"t1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data, _ := body["data"].(map[string]any)
	if data["access_token"] != "AT" {
		t.Fatalf("want tokens, got %v", body)
	}
	if _, leaked := body["callback_url"]; leaked {
		t.Fatalf("callback_url must not be returned to a mobile client: %v", body)
	}
}

// A step-up on the SECOND leg must behave like the first: a sealed token,
// no bearer tokens.
func TestMobileIDPComplete_StepUpSealsAPendingTokenAndIssuesNoTokens(t *testing.T) {
	h := mobileIDPHandler(t, func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
		return CompleteResult{EmailOTPRequired: true}, nil
	}).WithStepUp(&fakeCodeVerifier{}, &fakePendingStore{sealed: "sealed-value"})

	_, body := postMobileIDP(t, h.mobileIDPComplete, "/auth/zitadel/mobile/idp/complete",
		`{"auth_request_id":"V2_1","login_name":"a@b.test","session_id":"s1","session_token":"tok-1","workspace_tenant":"t1"}`)

	data, _ := body["data"].(map[string]any)
	if data["pending_token"] != "sealed-value" {
		t.Fatalf("want the sealed pending token, got %v", body)
	}
	if _, ok := data["access_token"]; ok {
		t.Fatal("tokens must NOT be issued while a step-up is outstanding")
	}
}

// The mobile routes sit behind the same X-Internal-Auth gate as every
// other Zitadel route: only marketplace-api may reach them.
func TestMobileIDPRoutes_RejectUnauthenticatedCallers(t *testing.T) {
	c := unreachableZitadel(t)
	h := NewHandler(c, nil).WithInternalAuth(testInternalSecret).WithGoogleIDPID(testGoogleIDPID)

	for name, fn := range map[string]func(http.ResponseWriter, *http.Request){
		"start":    h.idpStart,
		"finish":   h.mobileIDPFinish,
		"complete": h.mobileIDPComplete,
	} {
		t.Run(name, func(t *testing.T) {
			rec, _ := postMobileIDP(t, fn, "/auth/zitadel/mobile/idp/"+name, `{}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}
