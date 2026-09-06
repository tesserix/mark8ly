package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/teamproxy"
)

type fakeIDPBackend struct {
	gotStartProvider, gotStartReturnURL string
	authURL                             string
	startErr                            error

	gotFinishProvider, gotIntentID, gotIntentToken string
	finish                                         authbffclient.IDPFinishResult
	finishErr                                      error

	gotLoginName, gotSessionID, gotSessionToken, gotTenant string
	complete                                               authbffclient.LoginResult
	completeErr                                            error
	completeCalls                                          int
}

func (f *fakeIDPBackend) IDPStart(_ context.Context, provider, returnURL string) (string, error) {
	f.gotStartProvider, f.gotStartReturnURL = provider, returnURL
	return f.authURL, f.startErr
}

func (f *fakeIDPBackend) IDPFinish(_ context.Context, provider, intentID, intentToken string) (authbffclient.IDPFinishResult, error) {
	f.gotFinishProvider, f.gotIntentID, f.gotIntentToken = provider, intentID, intentToken
	return f.finish, f.finishErr
}

func (f *fakeIDPBackend) IDPComplete(_ context.Context, loginName, sessionID, sessionToken, workspaceTenant string) (authbffclient.LoginResult, error) {
	f.completeCalls++
	f.gotLoginName, f.gotSessionID, f.gotSessionToken, f.gotTenant = loginName, sessionID, sessionToken, workspaceTenant
	return f.complete, f.completeErr
}

const testIDPReturnURL = "https://admin.mark8ly.com/auth/idp/mobile"

func idpRouter(t *testing.T, lister TenantLister, backend MobileIDPBackend) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			AuthzMiddleware:  authz.NewMiddleware(authz.NewFakeClient(), nil),
			StoresMiddleware: func(c *gin.Context) { c.Next() },
		},
		ZitadelEnabled: true,
		DualIssuer:     true,
		TokenVerifier:  &auth.FakeVerifier{UserID: "unused"},
		IDPHandler:     NewMobileIDPHandler(lister, backend, testIDPReturnURL, nil),
	})
	return r
}

func postIDP(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/admin/auth/idp"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Data
}

// Both routes must be reachable with NO Authorization header — they are
// how a client obtains one.
func TestMobileIDPRoutesNeedNoBearerToken(t *testing.T) {
	backend := &fakeIDPBackend{authURL: "https://zitadel.test/idp/start?x=1"}
	r := idpRouter(t, &fakeTenantLister{}, backend)

	rec := postIDP(t, r, "/start", `{"provider":"google"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "https://zitadel.test/idp/start?x=1", decodeData(t, rec)["auth_url"])
}

// The return URL is CONFIGURATION, never client input: Zitadel does not
// validate successUrl at all, so a device-supplied one would put the
// allowlist alone between an attacker and a completed admin sign-in.
func TestMobileIDPStartBuildsTheReturnURLServerSide(t *testing.T) {
	backend := &fakeIDPBackend{authURL: "https://zitadel.test/idp/start"}
	r := idpRouter(t, &fakeTenantLister{}, backend)

	rec := postIDP(t, r, "/start", `{"provider":"google","return_url":"https://evil.example.com/steal"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, testIDPReturnURL, backend.gotStartReturnURL)
	require.Equal(t, "google", backend.gotStartProvider)
}

// Google is the only provider this surface trusts, and an unknown one must
// be refused before auth-bff is called at all.
func TestMobileIDPStartRejectsAnyProviderButGoogle(t *testing.T) {
	for _, provider := range []string{"apple", "facebook", "", "GOOGLE "} {
		t.Run("provider="+provider, func(t *testing.T) {
			backend := &fakeIDPBackend{authURL: "https://zitadel.test/idp/start"}
			r := idpRouter(t, &fakeTenantLister{}, backend)

			rec := postIDP(t, r, "/start", `{"provider":"`+provider+`"}`)

			if strings.TrimSpace(strings.ToLower(provider)) == "google" {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				return
			}
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Empty(t, backend.gotStartProvider, "auth-bff must not be called for an unsupported provider")
		})
	}
}

// The happy path: finish resolves the identity, the tenant is looked up by
// the VERIFIED email auth-bff returned, complete yields tokens, and the
// body is the exact shape /auth/login answers with.
func TestMobileIDPFinishResolvesTheTenantByEmailAndReturnsTokens(t *testing.T) {
	backend := &fakeIDPBackend{
		finish: authbffclient.IDPFinishResult{
			TenantRequired: true, SessionID: "s1", SessionToken: "tok-1", LoginName: "a@b.test",
		},
		complete: authbffclient.LoginResult{
			UID: "u1", Email: "a@b.test", TenantID: "t-1",
			AccessToken: "AT", RefreshToken: "RT", TokenType: "Bearer", ExpiresIn: 3600,
		},
	}
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1", Name: "Mumbai Spice Co", Role: "owner"}}}
	r := idpRouter(t, lister, backend)

	rec := postIDP(t, r, "/finish", `{"intent_id":"i1","intent_token":"it1"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	data := decodeData(t, rec)
	require.Equal(t, "AT", data["access_token"])
	require.Equal(t, "t-1", data["tenant_id"])
	require.Len(t, data["tenants"], 1)
	// The lookup keys on auth-bff's email, not anything the device sent.
	require.Equal(t, "a@b.test", lister.got)
	require.Equal(t, "a@b.test", backend.gotLoginName)
	require.Equal(t, "t-1", backend.gotTenant)
	require.Equal(t, "google", backend.gotFinishProvider)
}

// A step-up must come back as the SAME shape password login uses, so the
// app routes to its existing OTP screen with no new handling.
func TestMobileIDPFinishReturnsTheStepUpShapeWithNoTokens(t *testing.T) {
	backend := &fakeIDPBackend{
		finish: authbffclient.IDPFinishResult{
			TenantRequired: true, SessionID: "s1", SessionToken: "tok-1", LoginName: "a@b.test",
		},
		complete: authbffclient.LoginResult{
			Email: "a@b.test", TenantID: "t-1", EmailOTPRequired: true, PendingToken: "sealed",
		},
	}
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1"}}}
	r := idpRouter(t, lister, backend)

	rec := postIDP(t, r, "/finish", `{"intent_id":"i1","intent_token":"it1"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	data := decodeData(t, rec)
	require.Equal(t, true, data["email_otp_required"])
	require.Equal(t, "sealed", data["pending_token"])
	require.NotContains(t, data, "access_token")
}

// The person proved they own the Google account and still has no store.
// That is an actionable, specific refusal — and no session is minted.
func TestMobileIDPFinishRefusesWhenNoStoreExistsWithoutCompleting(t *testing.T) {
	backend := &fakeIDPBackend{
		finish: authbffclient.IDPFinishResult{
			TenantRequired: true, SessionID: "s1", SessionToken: "tok-1", LoginName: "a@b.test",
		},
	}
	r := idpRouter(t, &fakeTenantLister{result: nil}, backend)

	rec := postIDP(t, r, "/finish", `{"intent_id":"i1","intent_token":"it1"}`)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "no_store")
	require.Zero(t, backend.completeCalls, "no sign-in may be completed for an account with no tenant")
}

// auth-bff's refusal codes must survive as distinct, actionable answers.
// Flattening them into "temporarily unavailable" is the dead end #493 was.
func TestMobileIDPFinishMapsAuthBFFRefusalsToDistinctAnswers(t *testing.T) {
	cases := []struct {
		code       string
		status     int
		wantStatus int
		wantCode   string
	}{
		{"no_admin_account", http.StatusForbidden, http.StatusNotFound, "no_store"},
		{"email_not_verified", http.StatusUnauthorized, http.StatusUnauthorized, "email_not_verified"},
		{"email_ambiguous", http.StatusConflict, http.StatusConflict, "email_ambiguous"},
		{"unexpected_idp", http.StatusUnauthorized, http.StatusUnauthorized, "invalid_credentials"},
		{"invalid_intent", http.StatusUnauthorized, http.StatusUnauthorized, "invalid_credentials"},
		// Unknown codes and outages both mean "not the merchant's fault",
		// and must never read as a credential problem.
		{"zitadel_unavailable", http.StatusServiceUnavailable, http.StatusBadGateway, "auth_unavailable"},
		{"something_new", http.StatusInternalServerError, http.StatusBadGateway, "auth_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			backend := &fakeIDPBackend{finishErr: &authbffclient.IDPError{Code: tc.code, Status: tc.status}}
			r := idpRouter(t, &fakeTenantLister{}, backend)

			rec := postIDP(t, r, "/finish", `{"intent_id":"i1","intent_token":"it1"}`)

			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tc.wantCode)
		})
	}
}

// A transport failure is never a credential failure: a merchant told
// "couldn't sign you in" during an outage retries forever.
func TestMobileIDPFinishReportsATransportFailureAsUnavailable(t *testing.T) {
	backend := &fakeIDPBackend{finishErr: errors.New("dial tcp: connection refused")}
	r := idpRouter(t, &fakeTenantLister{}, backend)

	rec := postIDP(t, r, "/finish", `{"intent_id":"i1","intent_token":"it1"}`)

	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "auth_unavailable")
}

func TestMobileIDPFinishRequiresBothIntentFields(t *testing.T) {
	for _, body := range []string{`{}`, `{"intent_id":"i1"}`, `{"intent_token":"it1"}`} {
		backend := &fakeIDPBackend{}
		r := idpRouter(t, &fakeTenantLister{}, backend)
		rec := postIDP(t, r, "/finish", body)
		require.Equal(t, http.StatusBadRequest, rec.Code, body)
		require.Empty(t, backend.gotIntentID, "auth-bff must not be called for an incomplete request")
	}
}

// marketplace-api never sends a workspace_tenant on finish, so auth-bff
// must always answer tenant_required. Anything else means the contract
// changed, and guessing at it would mint a session for a tenant nobody
// chose.
func TestMobileIDPFinishRefusesAnUnexpectedFinishShape(t *testing.T) {
	backend := &fakeIDPBackend{finish: authbffclient.IDPFinishResult{TenantRequired: false}}
	r := idpRouter(t, &fakeTenantLister{}, backend)

	rec := postIDP(t, r, "/finish", `{"intent_id":"i1","intent_token":"it1"}`)

	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	require.Zero(t, backend.completeCalls)
}
