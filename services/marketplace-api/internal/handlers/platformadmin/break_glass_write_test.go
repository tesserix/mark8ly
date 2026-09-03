package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// stubBreakGlassRotator records what tenant it was asked to rotate.
type stubBreakGlassRotator struct {
	err       error
	gotTenant uuid.UUID
	calls     int
}

func (s *stubBreakGlassRotator) RotateOne(_ context.Context, tenantID uuid.UUID) error {
	s.calls++
	s.gotTenant = tenantID
	return s.err
}

// stubBreakGlassWriter implements platformadmin.BreakGlassWriter, recording
// every call so tests can assert what the handler asked it to do.
type stubBreakGlassWriter struct {
	mu sync.Mutex

	disableErr error
	enableErr  error
	clearErr   error
	clearN     int64

	disabledTenant uuid.UUID
	disabledReason string
	enabledTenant  uuid.UUID
	gotIPHash      []byte
}

func (s *stubBreakGlassWriter) Disable(_ context.Context, tenantID uuid.UUID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabledTenant = tenantID
	s.disabledReason = reason
	return s.disableErr
}

func (s *stubBreakGlassWriter) Enable(_ context.Context, tenantID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabledTenant = tenantID
	return s.enableErr
}

func (s *stubBreakGlassWriter) ClearIPLock(_ context.Context, ipHash []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotIPHash = ipHash
	return s.clearN, s.clearErr
}

// fakeRateLimiter records the key it was asked to Reset.
type fakeRateLimiter struct {
	mu      sync.Mutex
	resetOn []string
}

func (f *fakeRateLimiter) Reset(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetOn = append(f.resetOn, key)
}

func discardBreakGlassAudit(*gin.Context, uuid.UUID, audit.Event) error { return nil }

func breakGlassWriteRouter(
	rotator platformadmin.BreakGlassRotator,
	writer platformadmin.BreakGlassWriter,
	limiter platformadmin.BreakGlassRateLimiter,
	ipKey breakglass.HMACKey,
	emit func(*gin.Context, uuid.UUID, audit.Event) error,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBreakGlassWriteHandler(nil, writer, rotator, limiter, ipKey, emit, nil).Register(r.Group(""))
	return r
}

func doPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// The rotate response must carry metadata only — never a credential. This
// is the handler-side twin of the "RotateOne returns only an error"
// contract: even if a future change made the response richer by accident,
// this pins the exact key set the wire is allowed to carry.
func TestBreakGlassWrite_RotateResponseContainsNoCredentialMaterial(t *testing.T) {
	tid := uuid.New()
	rotator := &stubBreakGlassRotator{}
	r := breakGlassWriteRouter(rotator, &stubBreakGlassWriter{}, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/"+tid.String()+"/rotate", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, tid, rotator.gotTenant, "rotate must call RotateOne for the path tenant")

	body := rec.Body.String()
	for _, forbidden := range []string{"password", "totp", "secret", "blob"} {
		require.NotContainsf(t, strings.ToLower(body), forbidden,
			"rotate response must never contain %q — RotateOne returns only an error", forbidden)
	}

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.ElementsMatch(t, []string{"tenant_id", "rotated_at"}, keysOf(parsed),
		"rotate response must carry metadata only")
}

func TestBreakGlassWrite_RotateNotFoundReturns404(t *testing.T) {
	rotator := &stubBreakGlassRotator{err: breakglass.ErrNotFound}
	r := breakGlassWriteRouter(rotator, &stubBreakGlassWriter{}, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/"+uuid.New().String()+"/rotate", "")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// Disable must reject an empty reason (#404) — free text is required, not
// optional, because an audit row saying WHAT happened without WHY defeats
// the point of this surface.
func TestBreakGlassWrite_DisableRejectsEmptyReason(t *testing.T) {
	writer := &stubBreakGlassWriter{}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	tid := uuid.New()
	rec := doPost(r, "/admin/break-glass/"+tid.String()+"/disable", `{"reason":""}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, uuid.Nil, writer.disabledTenant, "the writer must never be called with an empty reason")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "reason_required", body["error"])
}

func TestBreakGlassWrite_DisableRejectsWhitespaceOnlyReason(t *testing.T) {
	writer := &stubBreakGlassWriter{}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	tid := uuid.New()
	rec := doPost(r, "/admin/break-glass/"+tid.String()+"/disable", `{"reason":"   "}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, uuid.Nil, writer.disabledTenant)
}

func TestBreakGlassWrite_DisableRejectsMissingBody(t *testing.T) {
	writer := &stubBreakGlassWriter{}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	tid := uuid.New()
	rec := doPost(r, "/admin/break-glass/"+tid.String()+"/disable", "")
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBreakGlassWrite_DisableSucceedsWithReason(t *testing.T) {
	writer := &stubBreakGlassWriter{}
	tid := uuid.New()
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/"+tid.String()+"/disable", `{"reason":"compromised laptop"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, tid, writer.disabledTenant)
	require.Equal(t, "compromised laptop", writer.disabledReason)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "compromised laptop", body["reason"])
}

func TestBreakGlassWrite_EnableSucceeds(t *testing.T) {
	writer := &stubBreakGlassWriter{}
	tid := uuid.New()
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/"+tid.String()+"/enable", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, tid, writer.enabledTenant)
}

func TestBreakGlassWrite_EnableNotFoundReturns404(t *testing.T) {
	writer := &stubBreakGlassWriter{enableErr: breakglass.ErrNotFound}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/"+uuid.New().String()+"/enable", "")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// The IP must be hashed with the SAME key the login path uses — this
// proves the handler calls breakglass.HMACIPHash rather than some other
// derivation, by recomputing the expected hash independently and
// comparing.
func TestBreakGlassWrite_ClearLockout_HashesWithTheSameKeyTheLoginPathUses(t *testing.T) {
	key := breakglass.HMACKey("shared-secret-key")
	writer := &stubBreakGlassWriter{clearN: 2}
	limiter := &fakeRateLimiter{}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, limiter, key, discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/clear-lockout", `{"ip":"203.0.113.7"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	expectedHash := breakglass.HMACIPHash(key, "203.0.113.7")
	require.Equal(t, expectedHash, writer.gotIPHash,
		"clear-lockout must hash the IP with breakglass.HMACIPHash under the SAME key the login path uses")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.EqualValues(t, 2, body["removed"])
}

// Clearing the DB lockout alone is not enough — the in-process
// LoginRateLimiter must also be reset, using the exact key shape the login
// handler computes (first 16 bytes of the ip_hash), or a previously
// locked IP served by the SAME pod stays stuck on the in-memory counter
// even after the durable lock is gone.
func TestBreakGlassWrite_ClearLockout_ResetsTheRateLimiterWithTheLoginPathKeyShape(t *testing.T) {
	key := breakglass.HMACKey("shared-secret-key")
	writer := &stubBreakGlassWriter{clearN: 1}
	limiter := &fakeRateLimiter{}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, limiter, key, discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/clear-lockout", `{"ip":"198.51.100.9"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	ipHash := breakglass.HMACIPHash(key, "198.51.100.9")
	wantKey := string(ipHash[:16])
	require.Equal(t, []string{wantKey}, limiter.resetOn,
		"the rate limiter must be Reset with the same key shape the login handler's rlKey computes")
}

func TestBreakGlassWrite_ClearLockout_NilRateLimiterDoesNotPanic(t *testing.T) {
	writer := &stubBreakGlassWriter{clearN: 1}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	require.NotPanics(t, func() {
		rec := doPost(r, "/admin/break-glass/clear-lockout", `{"ip":"192.0.2.1"}`)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestBreakGlassWrite_ClearLockout_RejectsEmptyIP(t *testing.T) {
	writer := &stubBreakGlassWriter{}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/clear-lockout", `{"ip":""}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Nil(t, writer.gotIPHash)
}

// The audit trail must never carry the raw IP — only its HMAC hash — even
// though the request body necessarily does.
func TestBreakGlassWrite_ClearLockout_AuditNeverCarriesRawIP(t *testing.T) {
	key := breakglass.HMACKey("shared-secret-key")
	writer := &stubBreakGlassWriter{clearN: 1}

	var captured audit.Event
	emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error {
		captured = ev
		return nil
	}
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, writer, nil, key, emit)

	const rawIP = "203.0.113.42"
	rec := doPost(r, "/admin/break-glass/clear-lockout", `{"ip":"`+rawIP+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	encoded, err := json.Marshal(captured)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), rawIP,
		"the audit event must never contain the plaintext IP")
}

func TestBreakGlassWrite_RotateFailsReturns500AndStillLogsNoCredential(t *testing.T) {
	rotator := &stubBreakGlassRotator{err: errors.New("secret manager unreachable")}
	r := breakGlassWriteRouter(rotator, &stubBreakGlassWriter{}, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	rec := doPost(r, "/admin/break-glass/"+uuid.New().String()+"/rotate", "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "secret manager unreachable",
		"the underlying error must not be echoed to the caller")
}

func TestBreakGlassWrite_InvalidTenantIDReturns400(t *testing.T) {
	r := breakGlassWriteRouter(&stubBreakGlassRotator{}, &stubBreakGlassWriter{}, nil, breakglass.HMACKey("k"), discardBreakGlassAudit)

	for _, path := range []string{"/rotate", "/disable", "/enable"} {
		rec := doPost(r, "/admin/break-glass/not-a-uuid"+path, `{"reason":"x"}`)
		require.Equal(t, http.StatusBadRequest, rec.Code, "path %s", path)
	}
}
