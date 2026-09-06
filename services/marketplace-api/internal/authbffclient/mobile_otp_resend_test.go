package authbffclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResendOTP_PostsOnlyThePendingTokenAndReturnsTheFreshOne(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	var gotSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSecret = r.Header.Get("X-Internal-Auth")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"sent":true,"pending_token":"resealed-2"}}`))
	}))
	defer srv.Close()

	got, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		ResendOTP(context.Background(), "original")
	require.NoError(t, err)

	// The MOBILE route: auth-bff's /auth/otp/resend resumes from the
	// pending cookie a native client does not have.
	require.Equal(t, "/auth/zitadel/mobile/otp/resend", gotPath)
	require.Equal(t, "s3cret", gotSecret)
	require.Equal(t, "original", gotBody["pending_token"])
	// No address on the wire: auth-bff reads it from the sealed token, and
	// offering one here would invite a later change to trust it — which
	// would make this a way to mail a code anywhere.
	require.NotContains(t, gotBody, "email")

	require.Equal(t, "resealed-2", got.PendingToken)
}

// 429 is the one failure whose remedy is "wait", and it must survive as a
// distinct sentinel all the way to the screen.
func TestResendOTP_RateLimitedIsItsOwnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		ResendOTP(context.Background(), "original")

	require.ErrorIs(t, err, ErrRateLimited)
	require.NotErrorIs(t, err, ErrInvalidCredentials)
}

func TestResendOTP_ExpiredChallengeIsInvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_challenge"}`))
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		ResendOTP(context.Background(), "expired")

	require.ErrorIs(t, err, ErrInvalidCredentials)
	require.NotErrorIs(t, err, ErrRateLimited)
}

// A 200 carrying no fresh token would leave the client verifying against
// the OLD challenge — a correct code failing for reasons nothing on screen
// could explain. Fail here instead.
func TestResendOTP_MissingPendingTokenIsAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"sent":true}}`))
	}))
	defer srv.Close()

	_, err := NewMobileLoginClient(srv.URL, "s3cret", srv.Client()).
		ResendOTP(context.Background(), "original")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRateLimited)
}
