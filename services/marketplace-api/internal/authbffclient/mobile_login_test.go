package authbffclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMobileLogin_PostsToAuthBFFWithInternalAuth(t *testing.T) {
	var gotPath, gotSecret string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSecret = r.Header.Get("X-Internal-Auth")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"uid":"u1","email":"a@b.test","tenant_id":"t1","access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3599}}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		Login(context.Background(), "a@b.test", "pw", "t1")
	require.NoError(t, err)

	require.Equal(t, "/auth/zitadel/mobile/login", gotPath)
	// auth-bff's login route is internet-reachable at auth.mark8ly.com and
	// its ONLY protection is this header. Dropping it would turn the proxy
	// into an open credential oracle.
	require.Equal(t, "s3cret", gotSecret)
	require.Equal(t, "a@b.test", gotBody["login_name"])
	require.Equal(t, "t1", gotBody["workspace_tenant"])
	// auth_request_id is deliberately absent — auth-bff mints it for the
	// mobile route, because a native client has no browser redirect.
	require.NotContains(t, gotBody, "auth_request_id")

	require.Equal(t, "AT", got.AccessToken)
	require.Equal(t, "RT", got.RefreshToken)
	require.Equal(t, "t1", got.TenantID)
	require.False(t, got.EmailOTPRequired)
}

// A fresh install is always an unrecognised device, so this is the normal
// first-login path. It must surface as a step-up, NOT as a failure and not
// as a login with empty tokens.
func TestMobileLogin_EmailOTPRequiredSurfacesAsAStepUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"uid":"u1","email":"a@b.test","tenant_id":"t1","email_otp_required":true,"mfa_required":false}}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		Login(context.Background(), "a@b.test", "pw", "t1")
	require.NoError(t, err, "a step-up is a normal outcome, not an error")
	require.True(t, got.EmailOTPRequired)
	require.Empty(t, got.AccessToken, "no token may be issued while a step-up is outstanding")
}

func TestMobileLogin_TOTPRequiredCarriesTheSessionHandles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totp_required":true,"session_id":"sid","session_token":"stok"}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		Login(context.Background(), "a@b.test", "pw", "t1")
	require.NoError(t, err)
	require.True(t, got.TOTPRequired)
	// The client needs these to complete the second step.
	require.Equal(t, "sid", got.SessionID)
	require.Equal(t, "stok", got.SessionToken)
}

// Bad credentials must stay a 401 with auth-bff's enumeration-safe body,
// not be flattened into a generic 502.
func TestMobileLogin_InvalidCredentialsSurfacesAsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_credentials"}`))
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		Login(context.Background(), "a@b.test", "bad", "t1")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestMobileLogin_UpstreamFailureIsDistinctFromBadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		Login(context.Background(), "a@b.test", "pw", "t1")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInvalidCredentials,
		"an auth-bff outage must never read as 'wrong password'")
}

// The TOTP gate answers OUTSIDE the data envelope — it is not a completed
// login — so the sealed token arrives top-level. Losing it there is the
// #686 item 2 lockout: the app gets `totp_required` with nothing to resume
// from and reports that the app needs an update.
func TestMobileLogin_TOTPGateSurfacesTheTopLevelPendingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"totp_required":true,"pending_token":"sealed-value"}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		Login(context.Background(), "a@b.test", "pw", "t1")
	require.NoError(t, err, "a step-up is a normal outcome, not an error")
	require.True(t, got.TOTPRequired)
	require.Equal(t, "sealed-value", got.PendingToken)
	require.Empty(t, got.AccessToken)
}

// VerifyTOTP must hit /mobile/totp/VERIFY. /mobile/totp is the web handler
// in token-issuing mode: it requires an auth_request_id auth-bff never
// returns to a device and a workspace_tenant the device cannot know, so
// calling it from here 400s every time.
func TestMobileVerifyTOTP_PostsTheSealedTokenToTheVerifyRoute(t *testing.T) {
	var gotPath, gotSecret string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSecret = r.Header.Get("X-Internal-Auth")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"uid":"u1","email":"a@b.test","tenant_id":"t1","access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3599}}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		VerifyTOTP(context.Background(), "sealed", "123456")
	require.NoError(t, err)

	require.Equal(t, "/auth/zitadel/mobile/totp/verify", gotPath)
	require.Equal(t, "s3cret", gotSecret)
	require.Equal(t, "sealed", gotBody["pending_token"])
	require.Equal(t, "123456", gotBody["code"])
	// Nothing identifying is offered: the sealed token is the binding, and
	// a field here would invite a later change to start trusting it.
	require.NotContains(t, gotBody, "login_name")
	require.NotContains(t, gotBody, "session_id")
	require.NotContains(t, gotBody, "workspace_tenant")

	require.Equal(t, "AT", got.AccessToken)
	require.Equal(t, "t1", got.TenantID)
}

// auth-bff answers a wrong code with 401, which the shared post() flattens
// into ErrInvalidCredentials. The handler is what turns that back into
// TOTP-specific copy; here the only requirement is that it stays a
// credential error and never a generic upstream failure.
func TestMobileVerifyTOTP_WrongCodeIsACredentialError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_totp"}`))
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		VerifyTOTP(context.Background(), "sealed", "000000")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}
