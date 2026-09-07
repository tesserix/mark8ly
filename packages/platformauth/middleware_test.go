package platformauth_test

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

	"github.com/mark8ly/platformauth"
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

func newRouter(t *testing.T, secret string, nonces platformauth.NonceStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(platformauth.RequirePlatformAuth(platformauth.AuthConfig{
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

type reqOpt func(*platformauth.SignatureInput)

func withoutOperator(in *platformauth.SignatureInput)   { in.Operator = "" }
func withoutCapability(in *platformauth.SignatureInput) { in.Capability = "" }

func signedRequest(t *testing.T, method, target string, body []byte, opts ...reqOpt) *http.Request {
	t.Helper()

	// Split target into Path and RawQuery separately rather than assigning
	// the whole target to Path — the canonicaliser signs them as distinct
	// fields, and no current test target carries a query string, but a
	// future one that does must not silently sign the wrong thing.
	parsed, err := url.Parse(target)
	require.NoError(t, err)

	in := platformauth.SignatureInput{
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

	sig, err := platformauth.Sign(testSecret, in)
	require.NoError(t, err)

	var rdr *bytes.Reader
	if body == nil {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	req.Header.Set(platformauth.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformauth.HeaderNonce, in.Nonce)
	req.Header.Set(platformauth.HeaderSignature, sig)
	if in.Operator != "" {
		req.Header.Set(platformauth.HeaderOperator, in.Operator)
	}
	if in.Capability != "" {
		req.Header.Set(platformauth.HeaderCapability, in.Capability)
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
	in := platformauth.SignatureInput{
		Method: http.MethodGet, Path: "/admin/ping",
		Timestamp: "1755000000", Nonce: uuid.NewString(),
		Operator: "op_7f3a", Capability: "audit.read",
	}
	sig, err := platformauth.Sign(testSecret, in)
	require.NoError(t, err)
	req.Header.Set(platformauth.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformauth.HeaderNonce, in.Nonce)
	req.Header.Set(platformauth.HeaderSignature, sig)

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
	r.Use(platformauth.RequirePlatformAuth(platformauth.AuthConfig{
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
	in := platformauth.SignatureInput{
		Method:     http.MethodGet,
		Path:       "/admin/ping",
		Timestamp:  strconv.FormatInt(futureTS.Unix(), 10),
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	sig, err := platformauth.Sign(testSecret, in)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set(platformauth.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformauth.HeaderNonce, in.Nonce)
	req.Header.Set(platformauth.HeaderSignature, sig)
	req.Header.Set(platformauth.HeaderOperator, in.Operator)
	req.Header.Set(platformauth.HeaderCapability, in.Capability)

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

	in := platformauth.SignatureInput{
		Method:     http.MethodGet,
		Path:       "/admin/ping",
		Timestamp:  "+1755859200",
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	sig, err := platformauth.Sign(testSecret, in)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set(platformauth.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformauth.HeaderNonce, in.Nonce)
	req.Header.Set(platformauth.HeaderSignature, sig)
	req.Header.Set(platformauth.HeaderOperator, in.Operator)
	req.Header.Set(platformauth.HeaderCapability, in.Capability)

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

	req := signedRequest(t, http.MethodGet, "/admin/ping", nil, func(in *platformauth.SignatureInput) {
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

	staleIn := platformauth.SignatureInput{
		Method: http.MethodGet, Path: "/admin/ping",
		Timestamp: "1755000000", Nonce: uuid.NewString(),
		Operator: "op_7f3a", Capability: "audit.read",
	}
	staleSig, err := platformauth.Sign(testSecret, staleIn)
	require.NoError(t, err)
	staleReq := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	staleReq.Header.Set(platformauth.HeaderTimestamp, staleIn.Timestamp)
	staleReq.Header.Set(platformauth.HeaderNonce, staleIn.Nonce)
	staleReq.Header.Set(platformauth.HeaderSignature, staleSig)
	staleRec := httptest.NewRecorder()
	newRouter(t, testSecret, newMemNonces()).ServeHTTP(staleRec, staleReq)

	oversizedRec := httptest.NewRecorder()
	newRouter(t, testSecret, newMemNonces()).ServeHTTP(oversizedRec,
		signedRequest(t, http.MethodGet, "/admin/ping", nil, func(in *platformauth.SignatureInput) {
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

// withCapability overrides the signed capability value.
func withCapability(v string) reqOpt {
	return func(in *platformauth.SignatureInput) { in.Capability = v }
}

// Capability PRESENCE is enforced; capability VALUE is not — pending #333.
// This test pins BOTH halves so the enforcement matrix cannot drift
// silently in either direction:
//
//   - the declared switch is off, and
//   - with it off, an arbitrary capability string admits a write.
//
// MUTATION: flip CapabilityValueChecked to true in middleware.go and the
// second half fails — with 403 capability_route_undeclared here, since
// "/admin/ping" is this test's own throwaway route and was never declared
// in RequiredWriteCapabilities (a real mounted write route with a matching
// declared-but-empty entry would instead fail with capability_insufficient,
// since "not.a.real.capability" != ""). Either way the write stops being
// admitted — which is the point. The constant is a real switch wired to
// the real check, not a marker; a future #333 implementer flips it and
// fills in RequiredWriteCapabilities' values rather than writing the gate
// from scratch.
func TestWriteCapabilityValueIsRecordedButNotEnforced(t *testing.T) {
	require.False(t, platformauth.CapabilityValueChecked,
		"value enforcement is off pending #333; if this is now on, the second half of this test and the note in middleware.go both need updating")

	r := newRouter(t, testSecret, newMemNonces())
	body := []byte(`{"x":1}`)

	// A capability nobody has ever defined, on an otherwise valid signed
	// write. It is admitted, and the value is carried through to the audit
	// context rather than checked against anything.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body,
		withCapability("not.a.real.capability")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// And the read path reports the value it recorded, unmodified.
	readRec := httptest.NewRecorder()
	r.ServeHTTP(readRec, signedRequest(t, http.MethodGet, "/admin/ping", nil,
		withCapability("not.a.real.capability")))
	require.Equal(t, http.StatusOK, readRec.Code)
	require.Contains(t, readRec.Body.String(), `"capability":"not.a.real.capability"`)
}

// boolPtr is a tiny helper for AuthConfig.CapabilityChecked, which is a
// *bool specifically so its zero value (nil) is distinguishable from an
// explicit false — see AuthConfig's doc comment in middleware.go.
func boolPtr(b bool) *bool { return &b }

// newRouterWithCapabilityConfig is newRouter's counterpart for the tests
// below: it lets a test override AuthConfig.CapabilityChecked and
// RequiredCapabilities, which newRouter deliberately does not expose —
// every other test in this file exercises the shipped, switch-off state.
// A nil checked argument leaves CapabilityChecked unset, exactly as every
// production caller (Register, cmd/marketplace-api/main.go) does.
func newRouterWithCapabilityConfig(t *testing.T, checked *bool, required map[string]string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(platformauth.RequirePlatformAuth(platformauth.AuthConfig{
		Secret:               testSecret,
		NonceStore:           newMemNonces(),
		Now:                  func() time.Time { return fixedNow },
		CapabilityChecked:    checked,
		RequiredCapabilities: required,
	}))
	r.POST("/admin/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

// TestAuthConfigCapabilityCheckedNilDefaultsToProductionOff proves the
// thing #364 requires above everything else: leaving CapabilityChecked
// unset — what every production caller does — behaves EXACTLY like the
// shipped CapabilityValueChecked=false constant, not like some other
// default. If this ever starts failing, either the fallback in
// RequirePlatformAuth broke or CapabilityValueChecked itself changed; both
// are shipping-blocking for #364.
func TestAuthConfigCapabilityCheckedNilDefaultsToProductionOff(t *testing.T) {
	require.False(t, platformauth.CapabilityValueChecked,
		"production default must be off; #364 must not flip this")

	r := newRouterWithCapabilityConfig(t, nil, nil)
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body,
		withCapability("literally.anything")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestCapabilityCheckedOnAdmitsMatchingCapability: switch ON, presented
// capability equals the declared required value -> admitted.
func TestCapabilityCheckedOnAdmitsMatchingCapability(t *testing.T) {
	required := map[string]string{
		platformauth.CapabilityKey(http.MethodPost, "/admin/ping"): "billing.trial.extend",
	}
	r := newRouterWithCapabilityConfig(t, boolPtr(true), required)
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body,
		withCapability("billing.trial.extend")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestCapabilityCheckedOnRefusesMismatchedCapability: switch ON,
// presented capability differs from the declared required value -> 403
// capability_insufficient. This is the "your capability is wrong" cell —
// distinguishable from the "this route declared nothing" cell below.
func TestCapabilityCheckedOnRefusesMismatchedCapability(t *testing.T) {
	required := map[string]string{
		platformauth.CapabilityKey(http.MethodPost, "/admin/ping"): "billing.trial.extend",
	}
	r := newRouterWithCapabilityConfig(t, boolPtr(true), required)
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body,
		withCapability("something.else")))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "capability_insufficient", errorCode(t, rec))
}

// TestCapabilityCheckedOnFailsClosedForUndeclaredRoute: switch ON, the
// write route has NO entry in the lookup at all -> refused with a code
// distinguishable from a capability mismatch. A missing declaration is a
// mark8ly bug (nothing to fix on the caller's side by presenting a
// different capability); a mismatch is a caller bug. Conflating them would
// send whoever is debugging a production refusal to the wrong place.
func TestCapabilityCheckedOnFailsClosedForUndeclaredRoute(t *testing.T) {
	r := newRouterWithCapabilityConfig(t, boolPtr(true), map[string]string{})
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body,
		withCapability("anything")))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "capability_route_undeclared", errorCode(t, rec))
	require.NotEqual(t, "capability_insufficient", errorCode(t, rec))
}

// newRouterWithReadCapabilityConfig is newRouter's counterpart for the read
// gate: it lets a test override AuthConfig.RequiredReadCaps, which newRouter
// deliberately does not expose. A nil required argument leaves
// RequiredReadCaps unset, exactly as every production caller does, and the
// route is mounted with the SAME GET method every read on this surface uses.
func newRouterWithReadCapabilityConfig(t *testing.T, required map[string]string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(platformauth.RequirePlatformAuth(platformauth.AuthConfig{
		Secret:           testSecret,
		NonceStore:       newMemNonces(),
		Now:              func() time.Time { return fixedNow },
		RequiredReadCaps: required,
	}))
	r.GET("/admin/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"operator":   c.GetString("platform_operator_id"),
			"capability": c.GetString("platform_capability"),
		})
	})
	return r
}

// TestDeclaredReadWithExactCapabilityIsAdmitted: a read route declared in
// RequiredReadCaps, presented with an operator and the exact required
// capability, is admitted.
func TestDeclaredReadWithExactCapabilityIsAdmitted(t *testing.T) {
	required := map[string]string{
		platformauth.CapabilityKey(http.MethodGet, "/admin/ping"): "rotate-credentials",
	}
	r := newRouterWithReadCapabilityConfig(t, required)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil,
		withCapability("rotate-credentials")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestDeclaredReadWithWrongCapabilityIsRefused: the same declared route,
// presented with "platform" — the value the console's audit module
// actually sends today — is refused. Exact string equality, no
// implication between capabilities (#275).
func TestDeclaredReadWithWrongCapabilityIsRefused(t *testing.T) {
	required := map[string]string{
		platformauth.CapabilityKey(http.MethodGet, "/admin/ping"): "rotate-credentials",
	}
	r := newRouterWithReadCapabilityConfig(t, required)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil,
		withCapability("platform")))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "capability_insufficient", errorCode(t, rec))
}

// TestDeclaredReadWithNoCapabilityHeaderIsRefused: the declared route,
// presented with an operator but no capability header at all, is refused
// 401 capability_required — not silently admitted.
func TestDeclaredReadWithNoCapabilityHeaderIsRefused(t *testing.T) {
	required := map[string]string{
		platformauth.CapabilityKey(http.MethodGet, "/admin/ping"): "rotate-credentials",
	}
	r := newRouterWithReadCapabilityConfig(t, required)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil, withoutCapability))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "capability_required", errorCode(t, rec))
}

// TestDeclaredReadWithCapabilityButNoOperatorIsRefused: the declared route,
// presented with a capability but no operator, is refused 401
// operator_required. Operator is checked first, mirroring the write
// branch's ordering.
func TestDeclaredReadWithCapabilityButNoOperatorIsRefused(t *testing.T) {
	required := map[string]string{
		platformauth.CapabilityKey(http.MethodGet, "/admin/ping"): "rotate-credentials",
	}
	r := newRouterWithReadCapabilityConfig(t, required)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil,
		withoutOperator, withCapability("rotate-credentials")))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "operator_required", errorCode(t, rec))
}

// TestUndeclaredReadRouteWithNoCapabilityAtAllStillSucceeds is the
// load-bearing assertion of this task: a read route that is ABSENT from
// RequiredReadCapabilities — every shipped read on this surface today,
// including /admin/audit-logs, /admin/health, and both billing reads — must
// keep working with no operator and no capability header whatsoever. This
// is the regression that would take down four routes already live in
// production if the read gate's undeclared branch ever changed to fail
// closed instead of proceeding.
func TestUndeclaredReadRouteWithNoCapabilityAtAllStillSucceeds(t *testing.T) {
	// Empty map, not nil: proves an explicitly-empty read map — the
	// production shape before break-glass's own entry is considered —
	// still lets an undeclared route straight through.
	r := newRouterWithReadCapabilityConfig(t, map[string]string{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil,
		withoutOperator, withoutCapability))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestRequiredReadCapabilitiesHasExactlyTheBreakGlassEntry pins the
// production map's shape: exactly one entry, keyed on the break-glass GET
// route's full template, requiring "rotate-credentials". A second entry
// appearing here without a corresponding update to this test is exactly
// the kind of silent scope creep #364's write-map coverage test guards
// against on the write side.
func TestRequiredReadCapabilitiesHasExactlyTheBreakGlassEntry(t *testing.T) {
	require.Len(t, platformauth.RequiredReadCapabilities, 1)

	key := platformauth.CapabilityKey(http.MethodGet, "/api/v1/platform/admin/break-glass")
	required, declared := platformauth.RequiredReadCapabilities[key]
	require.True(t, declared, "break-glass GET route must be declared in RequiredReadCapabilities")
	require.Equal(t, "rotate-credentials", required)
}

// TestAuthConfigRequiredReadCapsNilDefaultsToProductionMap mirrors
// TestAuthConfigCapabilityCheckedNilDefaultsToProductionOff: leaving
// RequiredReadCaps unset — what every production caller does — must behave
// exactly like falling back to the real RequiredReadCapabilities map, so a
// request against the real break-glass route template is gated even though
// this test never set RequiredReadCaps itself.
func TestAuthConfigRequiredReadCapsNilDefaultsToProductionMap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformauth.RequirePlatformAuth(platformauth.AuthConfig{
		Secret:     testSecret,
		NonceStore: newMemNonces(),
		Now:        func() time.Time { return fixedNow },
	}))
	r.GET("/api/v1/platform/admin/break-glass", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// No capability at all against the real, undeclared-in-test-but-real
	// production route: refused, because RequiredReadCapabilities really
	// does declare it even though this test passed nothing for
	// RequiredReadCaps.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/api/v1/platform/admin/break-glass", nil,
		withoutOperator, withoutCapability))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "operator_required", errorCode(t, rec))

	// And the exact required value admits it.
	okRec := httptest.NewRecorder()
	r.ServeHTTP(okRec, signedRequest(t, http.MethodGet, "/api/v1/platform/admin/break-glass", nil,
		withCapability("rotate-credentials")))
	require.Equal(t, http.StatusOK, okRec.Code, okRec.Body.String())
}
