package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

const testSecret = "test-platform-secret"

var fixedNow = time.Unix(1755859200, 0).UTC()

// memNonces is an in-memory NonceStore. Fine for middleware tests, which are
// about the enforcement matrix; Task 5 covers the real cross-replica store.
// It records the expiresAt it was last handed so tests can pin the TTL the
// middleware computes.
type memNonces struct {
	seen          map[string]bool
	lastExpiresAt time.Time
}

func newMemNonces() *memNonces { return &memNonces{seen: map[string]bool{}} }

func (m *memNonces) Claim(_ context.Context, nonce string, expiresAt time.Time) (bool, error) {
	m.lastExpiresAt = expiresAt
	if m.seen[nonce] {
		return false, nil
	}
	m.seen[nonce] = true
	return true, nil
}

func newRouter(t *testing.T, secret string, nonces platformadmin.NonceStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret:     secret,
		NonceStore: nonces,
		Now:        func() time.Time { return fixedNow },
	}))
	r.GET("/admin/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"operator":   c.GetString("platform_operator_id"),
			"capability": c.GetString("platform_capability"),
		})
	})
	r.POST("/admin/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

type reqOpt func(*platformadmin.SignatureInput)

func withoutOperator(in *platformadmin.SignatureInput)   { in.Operator = "" }
func withoutCapability(in *platformadmin.SignatureInput) { in.Capability = "" }

func signedRequest(t *testing.T, method, target string, body []byte, opts ...reqOpt) *http.Request {
	t.Helper()

	// Split target into Path and RawQuery separately rather than assigning
	// the whole target to Path — the canonicaliser signs them as distinct
	// fields, and no current test target carries a query string, but a
	// future one that does must not silently sign the wrong thing.
	parsed, err := url.Parse(target)
	require.NoError(t, err)

	in := platformadmin.SignatureInput{
		Method:     method,
		Path:       parsed.Path,
		RawQuery:   parsed.RawQuery,
		Body:       body,
		Timestamp:  "1755859200",
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	for _, o := range opts {
		o(&in)
	}

	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)

	var rdr *bytes.Reader
	if body == nil {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)
	if in.Operator != "" {
		req.Header.Set(platformadmin.HeaderOperator, in.Operator)
	}
	if in.Capability != "" {
		req.Header.Set(platformadmin.HeaderCapability, in.Capability)
	}
	return req
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Error
}

// Cell: secret unset -> 503 on every path. This surface fails CLOSED, unlike
// internalsvc.RequireInternalAuth which no-ops on an empty secret. An
// unconfigured deploy must be inert, not open.
func TestUnconfiguredSecretFailsClosed(t *testing.T) {
	r := newRouter(t, "", newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/ping", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}

// Cell: valid signature on a read -> allowed, context populated.
func TestValidReadIsAllowed(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "op_7f3a")
	require.Contains(t, rec.Body.String(), "audit.read")
}

// Cell: read without operator identity -> permitted (#275 acceptance).
func TestReadWithoutOperatorIsPermitted(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil, withoutOperator, withoutCapability))

	require.Equal(t, http.StatusOK, rec.Code)
}

// Cell: write without operator identity -> refused.
func TestWriteWithoutOperatorIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body, withoutOperator))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "operator_required", errorCode(t, rec))
}

// Cell: write without capability -> refused. Never inferred from the route.
func TestWriteWithoutCapabilityIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body, withoutCapability))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "capability_required", errorCode(t, rec))
}

func TestMissingSignatureIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/ping", nil))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "unauthenticated", errorCode(t, rec))
}

// Signature, timestamp and nonce failures share one code deliberately —
// distinguishing them tells an attacker which half of the check they
// passed.
func TestStaleTimestampIsRefusedWithOpaqueCode(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	req := signedRequest(t, http.MethodGet, "/admin/ping", nil)
	// Re-sign with a timestamp well outside the +/-300s window.
	in := platformadmin.SignatureInput{
		Method: http.MethodGet, Path: "/admin/ping",
		Timestamp: "1755000000", Nonce: uuid.NewString(),
		Operator: "op_7f3a", Capability: "audit.read",
	}
	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "unauthenticated", errorCode(t, rec))
}

func TestReplayedRequestIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	req := signedRequest(t, http.MethodGet, "/admin/ping", nil)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, req.Clone(context.Background()))
	require.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	r.ServeHTTP(second, req.Clone(context.Background()))
	require.Equal(t, http.StatusUnauthorized, second.Code)
	require.Equal(t, "unauthenticated", errorCode(t, second))
}

// The handler must still be able to read the body after the middleware has
// hashed it.
func TestBodyIsReadableDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret: testSecret, NonceStore: newMemNonces(),
		Now: func() time.Time { return fixedNow },
	}))
	r.POST("/admin/echo", func(c *gin.Context) {
		var payload map[string]any
		require.NoError(t, c.ShouldBindJSON(&payload))
		c.JSON(http.StatusOK, payload)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/echo", []byte(`{"hello":"world"}`)))

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"hello":"world"}`, rec.Body.String())
}

// Fix round 1, item 1: the nonce TTL must be anchored to the SIGNED
// timestamp, not to arrival time. A request signed at signedTS stays
// signature-valid across the whole window around it, including the
// future-dated edge — which can still arrive well inside tolerance. If the
// TTL were anchored to "now" instead, a sweeper (Task 9) could expire the
// row before the request itself stops being replayable.
func TestNonceTTLAnchoredToSignedTimestamp(t *testing.T) {
	nonces := newMemNonces()
	r := newRouter(t, testSecret, nonces)

	// Future-dated relative to fixedNow, but still inside the +/-5min window.
	futureTS := fixedNow.Add(4 * time.Minute)
	in := platformadmin.SignatureInput{
		Method:     http.MethodGet,
		Path:       "/admin/ping",
		Timestamp:  strconv.FormatInt(futureTS.Unix(), 10),
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)
	req.Header.Set(platformadmin.HeaderOperator, in.Operator)
	req.Header.Set(platformadmin.HeaderCapability, in.Capability)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	minExpiry := futureTS.Add(5 * time.Minute)
	require.False(t, nonces.lastExpiresAt.Before(minExpiry),
		"nonce TTL %s must be at or after signedTS+window %s", nonces.lastExpiresAt, minExpiry)
}

// Fix round 1, item 2: the method the HMAC covers (always upper-cased by
// CanonicalString) must be the same method write-enforcement classifies.
// gin's router happens to 404 a lowercase method before reaching a real
// handler, but that safety must not be the only thing standing between a
// lowercase write and being treated as a read.
func TestLowercaseWriteMethodStillRequiresOperator(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	body := []byte(`{"x":1}`)

	req := signedRequest(t, "post", "/admin/ping", body, withoutOperator)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "operator_required", errorCode(t, rec))
}

// Fix round 1, item 3: a signed timestamp with a leading '+' or '-' must be
// rejected rather than accepted as an alternate spelling of the same
// instant — two byte strings for one instant is a wart in a scheme
// published as the reference implementation for the console team.
func TestTimestampWithLeadingSignIsRejected(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	in := platformadmin.SignatureInput{
		Method:     http.MethodGet,
		Path:       "/admin/ping",
		Timestamp:  "+1755859200",
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)
	req.Header.Set(platformadmin.HeaderOperator, in.Operator)
	req.Header.Set(platformadmin.HeaderCapability, in.Capability)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "unauthenticated", errorCode(t, rec))
}

// Fix round 1, item 4: operator/capability are bounded at 256 bytes so a
// buggy or compromised gateway cannot write unbounded junk into the audit
// trail this surface exists to produce. Rejected through the SAME opaque
// unauthenticated path as every other failure here — no new oracle.
func TestOversizedOperatorIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	tooLong := strings.Repeat("a", 257)

	req := signedRequest(t, http.MethodGet, "/admin/ping", nil, func(in *platformadmin.SignatureInput) {
		in.Operator = tooLong
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "unauthenticated", errorCode(t, rec))
}

// Confirms item 4's new rejection path did not introduce a distinguishable
// response: every 401 this middleware can produce — missing signature,
// stale timestamp, oversized identity field, replay — must be byte-for-byte
// the same response body.
func TestAllUnauthenticatedBodiesAreByteIdentical(t *testing.T) {
	missingRec := httptest.NewRecorder()
	newRouter(t, testSecret, newMemNonces()).ServeHTTP(missingRec,
		httptest.NewRequest(http.MethodGet, "/admin/ping", nil))

	staleIn := platformadmin.SignatureInput{
		Method: http.MethodGet, Path: "/admin/ping",
		Timestamp: "1755000000", Nonce: uuid.NewString(),
		Operator: "op_7f3a", Capability: "audit.read",
	}
	staleSig, err := platformadmin.Sign(testSecret, staleIn)
	require.NoError(t, err)
	staleReq := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	staleReq.Header.Set(platformadmin.HeaderTimestamp, staleIn.Timestamp)
	staleReq.Header.Set(platformadmin.HeaderNonce, staleIn.Nonce)
	staleReq.Header.Set(platformadmin.HeaderSignature, staleSig)
	staleRec := httptest.NewRecorder()
	newRouter(t, testSecret, newMemNonces()).ServeHTTP(staleRec, staleReq)

	oversizedRec := httptest.NewRecorder()
	newRouter(t, testSecret, newMemNonces()).ServeHTTP(oversizedRec,
		signedRequest(t, http.MethodGet, "/admin/ping", nil, func(in *platformadmin.SignatureInput) {
			in.Operator = strings.Repeat("a", 257)
		}))

	r4 := newRouter(t, testSecret, newMemNonces())
	replayReq := signedRequest(t, http.MethodGet, "/admin/ping", nil)
	first := httptest.NewRecorder()
	r4.ServeHTTP(first, replayReq.Clone(context.Background()))
	require.Equal(t, http.StatusOK, first.Code)
	replayRec := httptest.NewRecorder()
	r4.ServeHTTP(replayRec, replayReq.Clone(context.Background()))

	require.Equal(t, http.StatusUnauthorized, missingRec.Code)
	require.Equal(t, http.StatusUnauthorized, staleRec.Code)
	require.Equal(t, http.StatusUnauthorized, oversizedRec.Code)
	require.Equal(t, http.StatusUnauthorized, replayRec.Code)

	require.Equal(t, missingRec.Body.String(), staleRec.Body.String())
	require.Equal(t, missingRec.Body.String(), oversizedRec.Body.String())
	require.Equal(t, missingRec.Body.String(), replayRec.Body.String())
}
