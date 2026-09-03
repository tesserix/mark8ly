//go:build integration

package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// End-to-end proof of #404 test intent 6: clearing a lockout must remove
// the DURABLE DB row AND reset the in-process LoginRateLimiter that the
// SAME pod's login path reads — otherwise a previously locked IP still
// gets refused, only now for a reason nothing on the console can see.
//
// Wires the real breakglass.Repository, a real *breakglass.LoginRateLimiter
// SHARED between the login handler (internal/handlers/admin) and the
// break-glass write handler (this package) — exactly as main.go must do
// in production, since two different LoginRateLimiter instances would
// never observe each other's state.
func TestIntegration_ClearLockout_RemovesDBRowAndResetsRateLimiterSoALockedIPCanRetry(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	secrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())
	limiter := breakglass.NewLoginRateLimiter()
	ipKey := breakglass.HMACKey("shared-hmac-key-for-this-test")

	tenantID := uuid.New()
	boot := breakglass.NewBootstrapper(repo, secrets, "test-project")
	require.NoError(t, boot.Provision(context.Background(), tenantID))

	loginHandler := admin.NewBreakGlassLoginHandler(admin.BreakGlassDeps{
		Repo:        repo,
		Secrets:     secrets,
		RateLimiter: limiter,
		IPHMACKey:   ipKey,
		Sessions:    authbffclient.StaticIssuer{},
	})
	writeHandler := platformadmin.NewBreakGlassWriteHandler(
		db, repo, breakglass.NewRotator(repo, secrets, nil, nil),
		limiter, ipKey, nil, nil,
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/admin/break-glass/login", loginHandler.Login)
	writeHandler.Register(r.Group(""))

	const clientIP = "203.0.113.55"

	// Fail LoginMaxFailures times with the WRONG password from the SAME
	// IP, tripping both the in-memory limiter and the durable DB lockout.
	for i := 0; i < breakglass.LoginMaxFailures; i++ {
		rec := loginFrom(t, r, clientIP, tenantID, "wrong-password", "000000")
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	ipHash := breakglass.HMACIPHash(ipKey, clientIP)
	locked, err := repo.IsIPLocked(context.Background(), ipHash)
	require.NoError(t, err)
	require.True(t, locked, "the durable lockout must be in effect after LoginMaxFailures failures")

	// The IP is now locked — even a request bearing the RIGHT credentials
	// is refused with 429, proving the lock (not just wrong creds) is
	// what's blocking it.
	acc, err := repo.GetByTenant(context.Background(), tenantID)
	require.NoError(t, err)
	blob, err := secrets.Fetch(context.Background(), acc.SecretPath)
	require.NoError(t, err)
	validCode, err := breakglass.TOTPCode(blob.TOTPSecret, time.Now())
	require.NoError(t, err)

	lockedResp := loginFrom(t, r, clientIP, tenantID, blob.Password, validCode)
	require.Equal(t, http.StatusTooManyRequests, lockedResp.Code,
		"the IP must be locked out even with correct credentials")

	// Clear the lockout through the write handler.
	clearBody, err := json.Marshal(map[string]string{"ip": clientIP})
	require.NoError(t, err)
	clearReq := httptest.NewRequest(http.MethodPost, "/admin/break-glass/clear-lockout", bytes.NewReader(clearBody))
	clearReq.Header.Set("Content-Type", "application/json")
	clearRec := httptest.NewRecorder()
	r.ServeHTTP(clearRec, clearReq)
	require.Equal(t, http.StatusOK, clearRec.Code)

	var clearResult struct {
		Removed int64 `json:"removed"`
	}
	require.NoError(t, json.Unmarshal(clearRec.Body.Bytes(), &clearResult))
	require.GreaterOrEqual(t, clearResult.Removed, int64(1))

	stillLocked, err := repo.IsIPLocked(context.Background(), ipHash)
	require.NoError(t, err)
	require.False(t, stillLocked, "the durable DB lockout row must be gone")

	// A fresh TOTP code — time may have moved past the 30s window.
	freshCode, err := breakglass.TOTPCode(blob.TOTPSecret, time.Now())
	require.NoError(t, err)
	retryResp := loginFrom(t, r, clientIP, tenantID, blob.Password, freshCode)
	require.Equal(t, http.StatusOK, retryResp.Code,
		"a previously locked IP must be able to log in again after clear-lockout — "+
			"both the durable lock AND the in-memory limiter must be clear")
}

// The DB clear alone is not sufficient proof that the in-memory limiter
// was also reset: IsIPLocked only ever consults the DB on the happy path,
// so a successful retry above would pass even if the in-memory counter
// were left untouched. This test isolates the in-memory half: if
// clear-lockout did NOT reset LoginRateLimiter, the stale failure count
// (already at LoginMaxFailures) plus one more wrong-password attempt
// would push RecordFailure's count past the threshold again and
// IMMEDIATELY re-persist a fresh DB lockout — even though the operator
// just cleared it moments ago. A properly reset counter leaves that one
// wrong attempt safely under threshold.
func TestIntegration_ClearLockout_ResetsInMemoryCounterNotJustTheDBRow(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	secrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())
	limiter := breakglass.NewLoginRateLimiter()
	ipKey := breakglass.HMACKey("shared-hmac-key-for-this-test-2")

	tenantID := uuid.New()
	boot := breakglass.NewBootstrapper(repo, secrets, "test-project")
	require.NoError(t, boot.Provision(context.Background(), tenantID))

	loginHandler := admin.NewBreakGlassLoginHandler(admin.BreakGlassDeps{
		Repo:        repo,
		Secrets:     secrets,
		RateLimiter: limiter,
		IPHMACKey:   ipKey,
		Sessions:    authbffclient.StaticIssuer{},
	})
	writeHandler := platformadmin.NewBreakGlassWriteHandler(
		db, repo, breakglass.NewRotator(repo, secrets, nil, nil),
		limiter, ipKey, nil, nil,
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/admin/break-glass/login", loginHandler.Login)
	writeHandler.Register(r.Group(""))

	const clientIP = "203.0.113.66"
	ipHash := breakglass.HMACIPHash(ipKey, clientIP)

	for i := 0; i < breakglass.LoginMaxFailures; i++ {
		rec := loginFrom(t, r, clientIP, tenantID, "wrong-password", "000000")
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	locked, err := repo.IsIPLocked(context.Background(), ipHash)
	require.NoError(t, err)
	require.True(t, locked)

	clearBody, err := json.Marshal(map[string]string{"ip": clientIP})
	require.NoError(t, err)
	clearReq := httptest.NewRequest(http.MethodPost, "/admin/break-glass/clear-lockout", bytes.NewReader(clearBody))
	clearReq.Header.Set("Content-Type", "application/json")
	clearRec := httptest.NewRecorder()
	r.ServeHTTP(clearRec, clearReq)
	require.Equal(t, http.StatusOK, clearRec.Code)

	stillLocked, err := repo.IsIPLocked(context.Background(), ipHash)
	require.NoError(t, err)
	require.False(t, stillLocked)

	// ONE more wrong-password attempt, immediately after the clear.
	rec := loginFrom(t, r, clientIP, tenantID, "still-wrong-password", "000000")
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	relocked, err := repo.IsIPLocked(context.Background(), ipHash)
	require.NoError(t, err)
	require.False(t, relocked,
		"a single wrong attempt right after clear-lockout must NOT immediately "+
			"re-trigger the hard lockout — that would mean the in-memory "+
			"failure counter was never actually reset, only the DB row was")
}

func loginFrom(t *testing.T, r *gin.Engine, clientIP string, tenantID uuid.UUID, password, totpCode string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
		"password":  password,
		"totp_code": totpCode,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/break-glass/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", clientIP)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
