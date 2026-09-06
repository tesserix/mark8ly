package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
)

func postResend(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/admin/auth/otp/resend", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func resendBody(t *testing.T, rec *httptest.ResponseRecorder) struct {
	Data struct {
		Sent         bool   `json:"sent"`
		PendingToken string `json:"pending_token"`
	} `json:"data"`
	Error string `json:"error"`
} {
	t.Helper()
	var body struct {
		Data struct {
			Sent         bool   `json:"sent"`
			PendingToken string `json:"pending_token"`
		} `json:"data"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

// THE property: the resend hands back a FRESH pending token, which the
// client must swap to. The code and the challenge expire together, so a
// resend that returned nothing would leave a correct code failing against
// a dead challenge.
func TestMobileResendOTP_ReturnsAFreshPendingToken(t *testing.T) {
	backend := &fakeLoginBackend{resendResult: authbffclient.ResendResult{PendingToken: "resealed-2"}}
	r := loginRouter(t, &fakeTenantLister{}, backend)

	rec := postResend(t, r, `{"pending_token":"original"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := resendBody(t, rec)
	require.True(t, body.Data.Sent)
	require.Equal(t, "resealed-2", body.Data.PendingToken)
	require.NotEqual(t, "original", body.Data.PendingToken)
	require.Equal(t, "original", backend.gotPendingToken)
}

// Unauthenticated, like /auth/login and /auth/otp/verify: it is part of
// obtaining a bearer token, not something a bearer token protects.
func TestMobileResendOTP_NeedsNoBearerToken(t *testing.T) {
	backend := &fakeLoginBackend{resendResult: authbffclient.ResendResult{PendingToken: "resealed-2"}}
	rec := postResend(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"original"}`)

	require.NotEqual(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, http.StatusOK, rec.Code)
}

// A spent budget keeps its own code all the way to the client. Told only
// "something went wrong" a merchant keeps tapping a button that cannot
// work until the window rolls.
func TestMobileResendOTP_RateLimitedKeepsItsOwnCode(t *testing.T) {
	backend := &fakeLoginBackend{resendErr: authbffclient.ErrRateLimited}
	rec := postResend(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"original"}`)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "rate_limited", resendBody(t, rec).Error)
	require.Contains(t, rec.Body.String(), "few minutes")
}

// An expired or forged challenge is not a rate limit and not an outage:
// only starting over helps.
func TestMobileResendOTP_ExpiredChallengeIsItsOwnCode(t *testing.T) {
	backend := &fakeLoginBackend{resendErr: authbffclient.ErrInvalidCredentials}
	rec := postResend(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"forged"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "invalid_challenge", resendBody(t, rec).Error)
}

// An auth-bff outage must NOT read as a spent budget — the advice
// ("wait a few minutes") would be wrong and the merchant would stop
// trying.
func TestMobileResendOTP_UpstreamFailureIsNotRateLimited(t *testing.T) {
	backend := &fakeLoginBackend{resendErr: errors.New("connection refused")}
	rec := postResend(t, loginRouter(t, &fakeTenantLister{}, backend), `{"pending_token":"original"}`)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "auth_unavailable", resendBody(t, rec).Error)
}

func TestMobileResendOTP_RequiresAPendingToken(t *testing.T) {
	backend := &fakeLoginBackend{}
	rec := postResend(t, loginRouter(t, &fakeTenantLister{}, backend), `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, backend.resendCalls, "nothing to resend against; auth-bff must not be called")
}
