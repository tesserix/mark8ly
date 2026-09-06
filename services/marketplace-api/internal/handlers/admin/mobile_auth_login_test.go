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

type fakeLoginBackend struct {
	gotEmail, gotTenant      string
	result                   authbffclient.LoginResult
	err                      error
	gotPendingToken, gotCode string
	otpResult                authbffclient.LoginResult
	otpErr                   error
	totpResult               authbffclient.LoginResult
	totpErr                  error
}

func (f *fakeLoginBackend) Login(_ context.Context, email, _, workspaceTenant string) (authbffclient.LoginResult, error) {
	f.gotEmail, f.gotTenant = email, workspaceTenant
	return f.result, f.err
}

func (f *fakeLoginBackend) VerifyOTP(_ context.Context, pendingToken, code string) (authbffclient.LoginResult, error) {
	f.gotPendingToken, f.gotCode = pendingToken, code
	return f.otpResult, f.otpErr
}

func (f *fakeLoginBackend) VerifyTOTP(_ context.Context, pendingToken, code string) (authbffclient.LoginResult, error) {
	f.gotPendingToken, f.gotCode = pendingToken, code
	return f.totpResult, f.totpErr
}

func loginRouter(t *testing.T, lister TenantLister, backend MobileLoginBackend) *gin.Engine {
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
		// RegisterAdminMobile disables the whole group without a verifier,
		// so the login route needs one mounted even though it does not use
		// it — signing in would be pointless with no authenticated route.
		TokenVerifier: &auth.FakeVerifier{UserID: "unused"},
		LoginHandler:  NewMobileLoginHandler(lister, backend, nil),
	})
	return r
}

func post(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/admin/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// The login route must be reachable with NO Authorization header — it is
// how a client gets one. Every other mobile route requires a bearer token.
func TestMobileLogin_NeedsNoBearerToken(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1", Name: "Mumbai Spice Co", Role: "owner"}}}
	backend := &fakeLoginBackend{result: authbffclient.LoginResult{
		UID: "u1", Email: "a@b.test", TenantID: "t-1", AccessToken: "AT", RefreshToken: "RT", ExpiresIn: 3599,
	}}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"pw"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			AccessToken string                       `json:"access_token"`
			TenantID    string                       `json:"tenant_id"`
			Tenants     []teamproxy.TenantMembership `json:"tenants"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "AT", body.Data.AccessToken)
	require.Equal(t, "t-1", body.Data.TenantID)

	// The tenant is resolved server-side from the EMAIL before
	// authenticating — the client cannot know it yet, and this mirrors the
	// web's resolveWorkspaceTenant.
	require.Equal(t, "t-1", backend.gotTenant)
	require.Equal(t, "a@b.test", backend.gotEmail)

	// Every membership comes back so the client can offer a switcher
	// rather than being silently pinned to whichever one we picked.
	require.Len(t, body.Data.Tenants, 1)
}

// A merchant on two tenants must not be silently pinned: we pick one to
// authenticate with, but the full list is returned.
func TestMobileLogin_MultipleTenantsAllReturned(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{
		{TenantID: "t-1", Name: "A", Role: "owner"},
		{TenantID: "t-2", Name: "B", Role: "staff"},
	}}
	backend := &fakeLoginBackend{result: authbffclient.LoginResult{AccessToken: "AT", TenantID: "t-1"}}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"pw"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			Tenants []teamproxy.TenantMembership `json:"tenants"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Tenants, 2)
}

// Zero tenants must NOT reach auth-bff at all: there is no workspace_tenant
// to authenticate against, and probing credentials for an address with no
// membership is needless exposure.
func TestMobileLogin_NoTenantsIsRefusedWithoutTouchingCredentials(t *testing.T) {
	backend := &fakeLoginBackend{}
	rec := post(t, loginRouter(t, &fakeTenantLister{result: nil}, backend), `{"email":"nobody@b.test","password":"pw"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, backend.gotEmail, "credentials must not be sent when there is no tenant to log into")
}

// A step-up is a normal outcome and must be reported as one, with no
// tokens. This is the COMMON first-login path on a fresh install.
func TestMobileLogin_EmailOTPRequiredIsReportedNotFailed(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1"}}}
	backend := &fakeLoginBackend{result: authbffclient.LoginResult{
		UID: "u1", Email: "a@b.test", TenantID: "t-1", EmailOTPRequired: true,
	}}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"pw"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			EmailOTPRequired bool   `json:"email_otp_required"`
			AccessToken      string `json:"access_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Data.EmailOTPRequired)
	require.Empty(t, body.Data.AccessToken)
}

func TestMobileLogin_BadCredentialsIs401(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1"}}}
	backend := &fakeLoginBackend{err: authbffclient.ErrInvalidCredentials}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"bad"}`)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// An auth-bff outage must not be reported as a wrong password: the
// merchant would keep retrying a correct credential forever.
func TestMobileLogin_UpstreamFailureIs502Not401(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1"}}}
	backend := &fakeLoginBackend{err: errors.New("auth-bff down")}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"pw"}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestMobileLogin_MissingFieldsIs400(t *testing.T) {
	r := loginRouter(t, &fakeTenantLister{}, &fakeLoginBackend{})
	require.Equal(t, http.StatusBadRequest, post(t, r, `{"email":"a@b.test"}`).Code)
	require.Equal(t, http.StatusBadRequest, post(t, r, `{"password":"pw"}`).Code)
}

// ---- email-OTP completion --------------------------------------------
//
// The COMMON first sign-in on mobile: a fresh install is always an
// unrecognised device, so a new phone lands here rather than getting
// tokens from the login call.

func postOTP(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/admin/auth/otp/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMobileOTP_CorrectCodeReturnsTokensWithoutABearer(t *testing.T) {
	backend := &fakeLoginBackend{otpResult: authbffclient.LoginResult{
		UID: "u1", Email: "a@b.test", TenantID: "t-1", AccessToken: "AT", RefreshToken: "RT", ExpiresIn: 3599,
	}}
	r := loginRouter(t, &fakeTenantLister{}, backend)

	rec := postOTP(t, r, `{"pending_token":"sealed","code":"123456"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
			TenantID    string `json:"tenant_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "AT", body.Data.AccessToken)
	require.Equal(t, "t-1", body.Data.TenantID)

	require.Equal(t, "sealed", backend.gotPendingToken)
	require.Equal(t, "123456", backend.gotCode)
	// No tenant lookup on this path: the login call already resolved it,
	// and repeating it would be a second round-trip for a value the client
	// already holds.
	require.Empty(t, backend.gotEmail)
}

func TestMobileOTP_WrongCodeIs401(t *testing.T) {
	backend := &fakeLoginBackend{otpErr: authbffclient.ErrInvalidCredentials}
	rec := postOTP(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"sealed","code":"000000"}`)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// After the code is accepted the user IS authenticated, so an upstream
// failure must not be reported as a bad code — they would retype a correct
// one forever.
func TestMobileOTP_UpstreamFailureIs502Not401(t *testing.T) {
	backend := &fakeLoginBackend{otpErr: errors.New("auth-bff down")}
	rec := postOTP(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"sealed","code":"123456"}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestMobileOTP_MissingFieldsIs400(t *testing.T) {
	r := loginRouter(t, &fakeTenantLister{}, &fakeLoginBackend{})
	require.Equal(t, http.StatusBadRequest, postOTP(t, r, `{"code":"123456"}`).Code)
	require.Equal(t, http.StatusBadRequest, postOTP(t, r, `{"pending_token":"sealed"}`).Code)
}

// The login response must carry the sealed state, or the OTP screen has
// nothing to resume from.
func TestMobileLogin_ReturnsThePendingToken(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1"}}}
	backend := &fakeLoginBackend{result: authbffclient.LoginResult{
		EmailOTPRequired: true, PendingToken: "sealed-value", TenantID: "t-1",
	}}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"pw"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			PendingToken     string `json:"pending_token"`
			EmailOTPRequired bool   `json:"email_otp_required"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Data.EmailOTPRequired)
	require.Equal(t, "sealed-value", body.Data.PendingToken)
}

// ---- TOTP completion --------------------------------------------------
//
// #686 item 2: a merchant with an authenticator app was locked OUT of the
// app entirely. The login answered `totp_required` with nothing to resume
// from, and the app rendered that as "this app version needs an update" —
// advice no update could ever satisfy, because the route did not exist.

func postTOTP(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/admin/auth/totp/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMobileTOTP_CorrectCodeReturnsTokensWithoutABearer(t *testing.T) {
	backend := &fakeLoginBackend{totpResult: authbffclient.LoginResult{
		UID: "u1", Email: "a@b.test", TenantID: "t-1", AccessToken: "AT", RefreshToken: "RT", ExpiresIn: 3599,
	}}
	r := loginRouter(t, &fakeTenantLister{}, backend)

	rec := postTOTP(t, r, `{"pending_token":"sealed","code":"123456"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
			TenantID    string `json:"tenant_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "AT", body.Data.AccessToken)
	require.Equal(t, "t-1", body.Data.TenantID)

	// Identity travels in the sealed token, exactly as on the OTP path.
	require.Equal(t, "sealed", backend.gotPendingToken)
	require.Equal(t, "123456", backend.gotCode)
	require.Empty(t, backend.gotEmail, "no tenant lookup: the sealed token already carries it")
}

// A wrong code must map to the TOTP-specific error. `invalid_credentials`
// would have the app show password copy on a screen with no password
// field on it.
func TestMobileTOTP_WrongCodeIsTOTPSpecific401(t *testing.T) {
	backend := &fakeLoginBackend{totpErr: authbffclient.ErrInvalidCredentials}
	rec := postTOTP(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"sealed","code":"000000"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "invalid_totp", body.Error)
}

// A six-digit code rolls every 30 seconds; telling a merchant a correct
// one was wrong sends them round a loop that cannot end.
func TestMobileTOTP_UpstreamFailureIs502Not401(t *testing.T) {
	backend := &fakeLoginBackend{totpErr: errors.New("auth-bff down")}
	rec := postTOTP(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"sealed","code":"123456"}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestMobileTOTP_MissingFieldsIs400(t *testing.T) {
	r := loginRouter(t, &fakeTenantLister{}, &fakeLoginBackend{})
	require.Equal(t, http.StatusBadRequest, postTOTP(t, r, `{"code":"123456"}`).Code)
	require.Equal(t, http.StatusBadRequest, postTOTP(t, r, `{"pending_token":"sealed"}`).Code)
}

// The TOTP gate on LOGIN must hand back the sealed token and NOT the raw
// session handles — the app has one resumption mechanism, and empty
// session_id/session_token would advertise a route that does not work.
func TestMobileLogin_TOTPGateReturnsPendingTokenNotSessionHandles(t *testing.T) {
	lister := &fakeTenantLister{result: []teamproxy.TenantMembership{{TenantID: "t-1"}}}
	backend := &fakeLoginBackend{result: authbffclient.LoginResult{
		TOTPRequired: true, PendingToken: "sealed-value",
	}}

	rec := post(t, loginRouter(t, lister, backend), `{"email":"a@b.test","password":"pw"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, true, body.Data["totp_required"])
	require.Equal(t, "sealed-value", body.Data["pending_token"])
	require.NotContains(t, body.Data, "session_id")
	require.NotContains(t, body.Data, "session_token")
	require.Empty(t, body.Data["access_token"])
}
