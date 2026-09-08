package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// sharedLimiterTestWriter and sharedLimiterTestRotator satisfy
// platformadmin's BreakGlassWriter and BreakGlassRotator interfaces
// without touching the DB — clear-lockout's only DB-shaped dependency is
// h.writer.ClearIPLock, which this stub answers directly. Rotate/disable/
// enable are never exercised by either test that uses these. Declared
// here (not integration-tagged) so both this file and
// break_glass_shared_limiter_integration_test.go can use them.
type sharedLimiterTestWriter struct{ clearN int64 }

func (w *sharedLimiterTestWriter) Disable(context.Context, uuid.UUID, string) error { return nil }
func (w *sharedLimiterTestWriter) Enable(context.Context, uuid.UUID) error          { return nil }
func (w *sharedLimiterTestWriter) ClearIPLock(context.Context, []byte) (int64, error) {
	return w.clearN, nil
}

type sharedLimiterTestRotator struct{}

func (sharedLimiterTestRotator) RotateOne(context.Context, uuid.UUID) error { return nil }

// TestSharedRateLimiter_ClearLockoutResetsALoginPathFailure is the
// unit-test-safe half of the shared-limiter proof (mark8ly#642): it does
// NOT stand up a database, so it runs in the normal `go test ./...` pass
// rather than requiring the integration tag — see
// break_glass_shared_limiter_integration_test.go for the full end-to-end
// version that drives admin.BreakGlassLoginHandler.Login itself (which
// needs a live DB for its IsIPLocked check before recordFailure ever
// runs).
//
// It records a failure the EXACT way BreakGlassLoginHandler.recordFailure
// does — h.deps.RateLimiter.RecordFailure(breakglass.LoginRateLimitKey(ipHash))
// — using the identical package-level helpers the login handler calls
// (not a re-implementation), then drives platformadmin.BreakGlassWriteHandler's
// REAL /admin/break-glass/clear-lockout HTTP handler and asserts the
// SAME shared *breakglass.LoginRateLimiter instance was reset.
//
// If mark8ly#642 regresses — the login side and the write side wired
// with two SEPARATE *breakglass.LoginRateLimiter instances instead of
// one shared instance — this test fails: the assertion checks the
// instance the "login path" (the RecordFailure call above) wrote to,
// and the write handler's Reset would land on a different map entirely.
func TestSharedRateLimiter_ClearLockoutResetsALoginPathFailure(t *testing.T) {
	ipHMACKey := breakglass.HMACKey("shared-limiter-unit-test-key")
	const callerIP = "198.51.100.23"
	ipHash := breakglass.HMACIPHash(ipHMACKey, callerIP)
	key := breakglass.LoginKey{IPHash: ipHash}

	sharedLimiter := breakglass.NewLoginRateLimiter()

	// --- "login path": the exact two calls break_glass_login.go's
	// recordFailure makes on a failed attempt. ---
	_, _ = sharedLimiter.RecordFailure(context.Background(), key)
	recorded, err := sharedLimiter.Count(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, 1, recorded, "test setup: the simulated login failure must be recorded")

	// --- write path: the REAL clear-lockout HTTP handler. ---
	writeHandler := platformadmin.NewBreakGlassWriteHandler(
		nil, // db — unused by clearLockout beyond what's passed to writer/rateLimiter
		&sharedLimiterTestWriter{clearN: 1},
		sharedLimiterTestRotator{},
		sharedLimiter, // the SAME instance — this is the fix under test
		ipHMACKey,
		nil,
		nil,
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	writeHandler.Register(r.Group("/"))

	body, err := json.Marshal(map[string]string{"ip": callerIP})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/break-glass/clear-lockout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	cleared, err := sharedLimiter.Count(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, 0, cleared,
		"clear-lockout did not reset the bucket the login path recorded a failure on — in "+
			"production this is mark8ly#642's failure mode: the durable DB lockout row clears, "+
			"the API reports success, and the in-memory limiter silently keeps refusing the IP, "+
			"because the login and write handlers were wired with two separate "+
			"*breakglass.LoginRateLimiter instances instead of one shared instance")
}
