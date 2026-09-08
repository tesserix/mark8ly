//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// sharedLimiterTestWriter and sharedLimiterTestRotator are declared in
// break_glass_shared_limiter_test.go (not integration-tagged) so both
// that file's DB-free test and this one can use them.

// TestSharedRateLimiter_LoginFailureVisibleToClearLockout is THE test
// that catches mark8ly#642's two-rate-limiter bug. It wires
// admin.BreakGlassLoginHandler and platformadmin.BreakGlassWriteHandler
// with the SAME *breakglass.LoginRateLimiter — exactly as
// cmd/marketplace-api/main.go now does — and drives BOTH through their
// real production HTTP handlers (not stand-ins for the rate-limit logic):
// a failed login records a failure on the shared limiter, and the write
// handler's clear-lockout must be able to see and reset that same bucket.
//
// If main.go ever regresses to constructing two separate
// *breakglass.LoginRateLimiter instances — one for BreakGlassDeps.RateLimiter,
// another for platformadmin.Deps.BreakGlassRateLimiter — this exact
// scenario reproduces mark8ly#642's failure mode in production: an
// operator runs clear-lockout, the durable break_glass_lockouts row is
// deleted, the API reports success, and the in-memory limiter (a
// DIFFERENT map nobody ever resets) keeps refusing the IP as if nothing
// happened. That is a miserable thing to debug because every signal says
// it worked.
//
// Requires the login side to run its real IsIPLocked query, which needs
// a live DB — see the package's other break_glass_login_integration_test.go
// for the same constraint. Do NOT run this against the shared
// TEST_DATABASE_URL concurrently with other integration suites.
func TestSharedRateLimiter_LoginFailureVisibleToClearLockout(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	secrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())
	ipHMACKey := breakglass.HMACKey("shared-limiter-test-key")

	// ONE instance, threaded to both handlers — the fix under test.
	sharedLimiter := breakglass.NewLoginRateLimiter()

	loginHandler := admin.NewBreakGlassLoginHandler(admin.BreakGlassDeps{
		Repo:        repo,
		Secrets:     secrets,
		RateLimiter: sharedLimiter,
		IPHMACKey:   ipHMACKey,
	})
	writeHandler := platformadmin.NewBreakGlassWriteHandler(
		db,
		&sharedLimiterTestWriter{clearN: 1},
		sharedLimiterTestRotator{},
		sharedLimiter,
		ipHMACKey,
		nil, // emit — no-op default
		nil, // logger — slog.Default()
	)

	gin.SetMode(gin.TestMode)
	loginRouter := gin.New()
	loginRouter.POST("/admin/break-glass/login", loginHandler.Login)
	writeRouter := gin.New()
	writeHandler.Register(writeRouter.Group("/"))

	const callerIP = "203.0.113.55"

	// 1. A failed login (nonexistent tenant — no account needs
	// provisioning) from callerIP. Login()'s recordFailure path calls
	// h.deps.RateLimiter.RecordFailure(breakglass.LoginRateLimitKey(ipHash))
	// on sharedLimiter.
	body, err := json.Marshal(map[string]string{
		"tenant_id": uuid.New().String(),
		"password":  "wrong-password",
		"totp_code": "000000",
	})
	require.NoError(t, err)
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/break-glass/login", bytes.NewReader(body))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("X-Forwarded-For", callerIP)
	loginRec := httptest.NewRecorder()
	loginRouter.ServeHTTP(loginRec, loginReq)
	require.Equal(t, http.StatusUnauthorized, loginRec.Code, "expected the uniform invalid_credentials failure")

	ipHash := breakglass.HMACIPHash(ipHMACKey, callerIP)
	key := breakglass.LoginKey{IPHash: ipHash}
	recorded, err := sharedLimiter.Count(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, 1, recorded,
		"the login path must have recorded its failure on sharedLimiter — if this is 0, "+
			"the login handler is not touching the instance the test wired it with")

	// 2. clear-lockout for the SAME ip, via the write handler's real HTTP
	// route. It must reach the SAME bucket the login path just wrote to.
	clearBody, err := json.Marshal(map[string]string{"ip": callerIP})
	require.NoError(t, err)
	clearReq := httptest.NewRequest(http.MethodPost, "/admin/break-glass/clear-lockout", bytes.NewReader(clearBody))
	clearReq.Header.Set("Content-Type", "application/json")
	clearRec := httptest.NewRecorder()
	writeRouter.ServeHTTP(clearRec, clearReq)
	require.Equal(t, http.StatusOK, clearRec.Code)

	cleared, err := sharedLimiter.Count(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, 0, cleared,
		"clear-lockout did NOT reset the same in-memory bucket the login path recorded the "+
			"failure on. In production this is mark8ly#642's failure mode: the durable DB "+
			"lockout clears, the API reports success, and the in-memory limiter silently keeps "+
			"refusing the IP — because the login and write handlers were wired with two "+
			"separate *breakglass.LoginRateLimiter instances instead of one shared instance.")
}
