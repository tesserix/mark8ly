package authbffclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyOTP_PostsPendingTokenAndCode(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"uid":"u1","email":"a@b.test","tenant_id":"t1","access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3599}}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		VerifyOTP(context.Background(), "sealed-token", "123456")
	require.NoError(t, err)

	require.Equal(t, "/auth/zitadel/mobile/otp/verify", gotPath)
	require.Equal(t, "sealed-token", gotBody["pending_token"])
	require.Equal(t, "123456", gotBody["code"])
	// The email is deliberately NOT sent: auth-bff takes identity from the
	// sealed token, and offering one here invites a later change to start
	// trusting it — the exact binding the challenge exists to protect.
	require.NotContains(t, gotBody, "email")

	require.Equal(t, "AT", got.AccessToken)
	require.Equal(t, "t1", got.TenantID)
}

// A wrong code stays 401 so the screen can say "that code is wrong" rather
// than "something went wrong".
func TestVerifyOTP_WrongCodeIsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_code"}`))
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		VerifyOTP(context.Background(), "sealed", "000000")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

// Once the code is accepted the user IS authenticated, so a server-side
// failure must not read as a bad code — otherwise they retry a correct one
// indefinitely.
func TestVerifyOTP_ServerFailureIsNotABadCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		VerifyOTP(context.Background(), "sealed", "123456")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInvalidCredentials)
}

// The login response's pending_token must survive into the result, or the
// client has no way to resume the challenge.
func TestLogin_CarriesThePendingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"uid":"u1","email":"a@b.test","tenant_id":"t1","email_otp_required":true,"pending_token":"sealed-value"}}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "", srv.Client()).
		Login(context.Background(), "a@b.test", "pw", "t1")
	require.NoError(t, err)
	require.True(t, got.EmailOTPRequired)
	require.Equal(t, "sealed-value", got.PendingToken)
	require.Empty(t, got.AccessToken)
}
