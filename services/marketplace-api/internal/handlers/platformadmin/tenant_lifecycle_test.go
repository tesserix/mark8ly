package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
)

// testTenant is a fixed, valid UUID used across this file's fixtures, so
// the golden fixture and the assertions cannot drift apart.
const testTenant = "44444444-4444-4444-4444-444444444444"

// stubLifecycle records what it was asked to do and returns canned results.
// Values are DISTINCT and NON-ZERO so an assertion cannot pass on a
// fabricated zero from a missing field.
type stubLifecycle struct {
	res       *tenantlifecycle.Result
	err       error
	gotTenant string
	calls     int
}

func (s *stubLifecycle) Suspend(_ context.Context, id string) (*tenantlifecycle.Result, error) {
	s.calls++
	s.gotTenant = id
	return s.res, s.err
}
func (s *stubLifecycle) Unsuspend(_ context.Context, id string) (*tenantlifecycle.Result, error) {
	s.calls++
	s.gotTenant = id
	return s.res, s.err
}

// fakeLifecycleStoreRepo implements stores.Repository for this file's unit
// tests. Only the two methods this handler calls (SuspendActiveForTenant,
// MarkStaleForTenant) record anything; the rest are inert stubs, mirroring
// fakeStoreRepo in internal/subscription/planchange/preflight_test.go.
type fakeLifecycleStoreRepo struct {
	mu               sync.Mutex
	suspendedTenants []string
	staleTenants     []string
}

func (r *fakeLifecycleStoreRepo) GetByIDForTenant(_ context.Context, _, _ string) (*stores.Store, error) {
	return nil, stores.ErrNotFound
}
func (r *fakeLifecycleStoreRepo) GetBySlug(_ context.Context, _ string) (*stores.Store, error) {
	return nil, stores.ErrNotFound
}
func (r *fakeLifecycleStoreRepo) ListForTenant(_ context.Context, _ string) ([]stores.Store, error) {
	return nil, nil
}
func (r *fakeLifecycleStoreRepo) Upsert(_ context.Context, _ *stores.Store) error { return nil }
func (r *fakeLifecycleStoreRepo) GetProductsWatermark(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (r *fakeLifecycleStoreRepo) CountActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *fakeLifecycleStoreRepo) CountActiveOrSoftDeletedRestorableTx(_ context.Context, _ *gorm.DB, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *fakeLifecycleStoreRepo) ListActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) ([]stores.Store, error) {
	return nil, nil
}
func (r *fakeLifecycleStoreRepo) InFlightOrderCount(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *fakeLifecycleStoreRepo) SuspendActiveForTenant(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suspendedTenants = append(r.suspendedTenants, tenantID)
	return nil
}
func (r *fakeLifecycleStoreRepo) MarkStaleForTenant(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.staleTenants = append(r.staleTenants, tenantID)
	return nil
}

// newLifecycleDeps builds a handler wired to client, an inert local
// store repo, and an audit func that discards every event. Used by tests
// that don't care about the audit side effect.
func newLifecycleDeps(t *testing.T, client platformadmin.TenantLifecycle) *platformadmin.TenantLifecycleHandler {
	t.Helper()
	return platformadmin.NewTenantLifecycleHandler(
		client,
		&fakeLifecycleStoreRepo{},
		nil, // NewTenantLifecycleHandler treats nil as a no-op emit func
		nil,
	)
}

// newLifecycleDepsCapturingAudit builds a handler whose audit emission is
// captured synchronously into the returned slice, rather than routed
// through the real *audit.Emitter's async worker goroutine — the raw
// audit.Event is available at the call boundary itself, so no wait is
// needed for the assertion that follows.
func newLifecycleDepsCapturingAudit(t *testing.T, client platformadmin.TenantLifecycle) (*platformadmin.TenantLifecycleHandler, *[]audit.Event) {
	t.Helper()
	emitted := &[]audit.Event{}
	var mu sync.Mutex
	h := platformadmin.NewTenantLifecycleHandler(
		client,
		&fakeLifecycleStoreRepo{},
		func(_ *gin.Context, tenantID uuid.UUID, ev audit.Event) error {
			mu.Lock()
			defer mu.Unlock()
			ev.TenantID = tenantID
			*emitted = append(*emitted, ev)
			return nil
		},
		nil,
	)
	return h, emitted
}

// postLifecycle signs and sends a POST to /admin/tenants/{testTenant}/{action}
// through RequirePlatformAuth, mirroring newRouter/signedRequest in
// middleware_test.go. The request must carry a valid signature, an
// operator, and a capability, or it is rejected before the handler runs.
func postLifecycle(t *testing.T, h *platformadmin.TenantLifecycleHandler, action, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postLifecycleTenant(t, h, testTenant, action, body)
}

// postLifecycleTenant is postLifecycle parameterised on the tenant id, for
// the integration tests (Step 5), which need a real, per-test tenant id
// rather than the shared fixture constant.
func postLifecycleTenant(t *testing.T, h *platformadmin.TenantLifecycleHandler, tenantID, action, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret:     testSecret,
		NonceStore: newMemNonces(),
		Now:        func() time.Time { return fixedNow },
	}))
	h.Register(r.Group(""))

	target := "/admin/tenants/" + tenantID + "/" + action
	req := signedRequest(t, http.MethodPost, target, []byte(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestSuspend_RequiresKnownReasonCode(t *testing.T) {
	for _, body := range []string{
		`{}`,                                  // missing
		`{"reason_code":""}`,                  // empty
		`{"reason_code":"because_i_said_so"}`, // not in the set
		`{"reason":"free text only"}`,         // free text is not a substitute
	} {
		rec := postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{}), "suspend", body)
		require.Equal(t, http.StatusBadRequest, rec.Code, "body %s must be rejected", body)
		require.Contains(t, rec.Body.String(), "reason_code")
	}
}

func TestSuspend_AcceptsEveryDeclaredCode(t *testing.T) {
	for _, code := range platformadmin.SuspendReasonCodes {
		stub := &stubLifecycle{res: &tenantlifecycle.Result{
			TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
		rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend",
			`{"reason_code":"`+code+`"}`)
		require.Equal(t, http.StatusOK, rec.Code, "declared code %q must be accepted", code)
	}
}

// The upstream's result is projected, not passed through, and the counts
// come from upstream rather than being invented locally.
func TestSuspend_ProjectsUpstreamResult(t *testing.T) {
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
	rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend", `{"reason_code":"abuse"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, testTenant, stub.gotTenant)
	var body struct {
		Data struct {
			TenantID       string `json:"tenant_id"`
			Status         string `json:"status"`
			StoresAffected int    `json:"stores_affected"`
			Changed        bool   `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 3, body.Data.StoresAffected)
	require.True(t, body.Data.Changed)
	require.Equal(t, "suspended", body.Data.Status)
}

// An upstream ErrUnavailable must NOT read as "nothing to do".
func TestSuspend_UpstreamUnavailableIs503NotEmptySuccess(t *testing.T) {
	stub := &stubLifecycle{err: tenantlifecycle.ErrUnavailable}
	rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), `"changed"`,
		"a failed suspension must not shape a result at all")
}

func TestSuspend_UpstreamNotFoundIs404_ConflictIs409(t *testing.T) {
	rec := postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{err: tenantlifecycle.ErrNotFound}),
		"suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{err: tenantlifecycle.ErrConflict}),
		"suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusConflict, rec.Code)
}

// The audit row must carry the tenant, the operator, and the reason code —
// and there must be exactly ONE per changed call, none for a no-op.
func TestSuspend_AuditsOncePerChangeAndNeverForNoOp(t *testing.T) {
	changed := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
	deps, emitted := newLifecycleDepsCapturingAudit(t, changed)
	postLifecycle(t, deps, "suspend", `{"reason_code":"abuse","reason":"spam orders"}`)

	require.Len(t, *emitted, 1, "exactly one audit row per changed suspension")
	ev := (*emitted)[0]
	require.Equal(t, testTenant, ev.TenantID.String(),
		"an event with no tenant is silently DROPPED — this is the assertion that catches it")
	require.Equal(t, "abuse", ev.Metadata["reason_code"])
	require.Equal(t, "spam orders", ev.Metadata["reason"])
	require.Equal(t, 3, ev.Metadata["stores_affected"])

	noop := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 0, Changed: false}}
	deps2, emitted2 := newLifecycleDepsCapturingAudit(t, noop)
	rec := postLifecycle(t, deps2, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, *emitted2, "a no-op writes NO audit row (#287 acceptance)")
}

// The local projection is updated in the same request, so enforcement does
// not wait out StoreMiddleware's 5-minute TTL. The unit-level slice of
// this — that the handler calls SuspendActiveForTenant on a changed
// suspend and MarkStaleForTenant on a changed unsuspend, never the
// reverse — is covered here; the DB-backed proof that a real row's status
// actually changes lives in the integration test (Step 5).
func TestSuspend_UpdatesLocalProjectionImmediately(t *testing.T) {
	repo := &fakeLifecycleStoreRepo{}
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, repo, nil, nil)
	rec := postLifecycle(t, h, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{testTenant}, repo.suspendedTenants,
		"suspend must eagerly flip the local projection to suspended")
	require.Empty(t, repo.staleTenants, "suspend must never mark rows stale")
}

// Unsuspend takes effect on the next refresh, not instantly — see the type
// doc on TenantLifecycleHandler for why an eager local flip back to
// active would under-enforce.
func TestUnsuspend_MarksLocalProjectionStaleNotActive(t *testing.T) {
	repo := &fakeLifecycleStoreRepo{}
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "active", StoresAffected: 3, Changed: true}}
	h := platformadmin.NewTenantLifecycleHandler(stub, repo, nil, nil)
	rec := postLifecycle(t, h, "unsuspend", `{"reason_code":"resolved"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{testTenant}, repo.staleTenants,
		"unsuspend must mark the local projection stale so it is refetched")
	require.Empty(t, repo.suspendedTenants, "unsuspend must never call the suspend-projection path")
}

func TestUnsuspend_RequiresKnownReasonCode(t *testing.T) {
	rec := postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{}), "unsuspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"a suspend-only code must not be accepted on unsuspend — the two closed sets are different")
	require.Contains(t, rec.Body.String(), "reason_code")
}

func TestUnsuspend_AcceptsEveryDeclaredCode(t *testing.T) {
	for _, code := range platformadmin.UnsuspendReasonCodes {
		stub := &stubLifecycle{res: &tenantlifecycle.Result{
			TenantID: testTenant, Status: "active", StoresAffected: 2, Changed: true}}
		rec := postLifecycle(t, newLifecycleDeps(t, stub), "unsuspend",
			`{"reason_code":"`+code+`"}`)
		require.Equal(t, http.StatusOK, rec.Code, "declared code %q must be accepted", code)
	}
}

// THE test. Real handler output compared to the committed contract.
func TestSuspend_MatchesGoldenFixture(t *testing.T) {
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
	rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend", `{"reason_code":"abuse","reason":"spam orders"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/tenant_suspend_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}
