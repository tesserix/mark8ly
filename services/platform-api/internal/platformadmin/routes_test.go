package platformadmin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/platformadmin"
	"github.com/mark8ly/platformauth"
)

const testSecret = "test-platform-api-secret"

// memNonces is an in-memory platformauth.NonceStore, sufficient for these
// enforcement-matrix tests — no real database needed since the negative
// control and the signature checks below never exercise replay across
// process boundaries.
type memNonces struct{ seen map[string]bool }

func newMemNonces() *memNonces { return &memNonces{seen: map[string]bool{}} }

func (m *memNonces) Claim(_ context.Context, nonce string, _ time.Time) (bool, error) {
	if m.seen[nonce] {
		return false, nil
	}
	m.seen[nonce] = true
	return true, nil
}

func newTestRouter(t *testing.T, secret string, nonces platformauth.NonceStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), platformadmin.Deps{
		Secret:     secret,
		NonceStore: nonces,
	})
	return r
}

// signedRequest builds an httptest.Request carrying a valid platformauth
// signature for the given method/path, so tests can prove the positive
// path (not just rejections) actually works end to end.
func signedRequest(t *testing.T, secret, method, path string) *http.Request {
	t.Helper()

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := uuid.NewString()

	in := platformauth.SignatureInput{
		Method:    method,
		Path:      path,
		Timestamp: ts,
		Nonce:     nonce,
	}
	sig, err := platformauth.Sign(secret, in)
	require.NoError(t, err)

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(platformauth.HeaderTimestamp, ts)
	req.Header.Set(platformauth.HeaderNonce, nonce)
	req.Header.Set(platformauth.HeaderSignature, sig)
	return req
}

// TestNegativeControl is the acceptance criterion from #720: a made-up
// path under the same prefix must answer 404 (proving the router itself
// has no route there, as expected) while the REAL route, hit unsigned,
// answers 401 — not 404. Both landing on 404 would mean this surface was
// never mounted at all, which is indistinguishable from success in a
// smoke test that only asserts "not 200" — the exact silent failure #720
// names. Driven through router.ServeHTTP on the real router, not a
// GET-on-a-POST-route 405-vs-404 probe: HandleMethodNotAllowed defaults to
// false in every service here, so an unmatched method 404s whether or not
// the route is mounted, and would not distinguish the two cases.
func TestNegativeControl(t *testing.T) {
	r := newTestRouter(t, testSecret, newMemNonces())

	t.Run("real route unsigned answers 401, not 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, platformadmin.MountPrefix+"/admin/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code,
			"the real, mounted route must reject an unsigned request with 401")
	})

	t.Run("made-up path under the same prefix answers 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, platformadmin.MountPrefix+"/admin/this-route-does-not-exist", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code,
			"an unmounted path must 404 — if this were also 401 the middleware would be swallowing routing")
	})
}

// TestSignedRequestReachesHandler proves the positive path: a correctly
// signed request actually reaches the health handler and gets a 200, not
// just that unsigned/wrong requests get rejected. Without this, a
// middleware bug that rejects EVERY request (including validly signed
// ones) would still pass TestNegativeControl.
func TestSignedRequestReachesHandler(t *testing.T) {
	r := newTestRouter(t, testSecret, newMemNonces())

	req := signedRequest(t, testSecret, http.MethodGet, platformadmin.MountPrefix+"/admin/health")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "a correctly signed request must reach the handler: %s", w.Body.String())
}

// TestNotConfigured_MissingSecretAnswers503 matches marketplace-api's
// fail-closed posture: an empty Secret leaves the surface mounted but
// inert rather than open, so a deploy that ships before the secret exists
// refuses every request instead of accepting unsigned ones.
func TestNotConfigured_MissingSecretAnswers503(t *testing.T) {
	r := newTestRouter(t, "", newMemNonces())

	req := httptest.NewRequest(http.MethodGet, platformadmin.MountPrefix+"/admin/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
