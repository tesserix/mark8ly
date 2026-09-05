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
