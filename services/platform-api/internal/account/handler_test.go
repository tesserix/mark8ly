package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeSvc is a Service double that records the tenantID/actorUID it was
// called with and returns whatever err is configured, so tests can drive
// the handler without a real DB/FGA/GIP.
type fakeSvc struct {
	err            error
	calledTenantID string
	calledActorUID string
	called         bool
}

func (f *fakeSvc) DeleteAccount(_ context.Context, tenantID, actorUID string) error {
	f.called = true
	f.calledTenantID = tenantID
	f.calledActorUID = actorUID
	return f.err
}

// PurgeTenant satisfies the widened accountDeleter interface. Unused by
// the DeleteAccount-focused tests in this file — the teardown-route tests
// use fakePurger instead.
func (f *fakeSvc) PurgeTenant(_ context.Context, _ string, _ []string) (*PurgeResult, error) {
	return nil, nil
}

func newTestRouter(svc accountDeleter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	internal := r.Group("/internal")
	NewHandler(svc).Register(internal)
	return r
}

func TestAccountHandler_Delete_204(t *testing.T) {
	svc := &fakeSvc{}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":"owner-1"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !svc.called {
		t.Fatal("expected service.DeleteAccount to be called")
	}
	if svc.calledTenantID != "t1" {
		t.Fatalf("tenantID=%q, want t1", svc.calledTenantID)
	}
	if svc.calledActorUID != "owner-1" {
		t.Fatalf("actorUID=%q, want owner-1", svc.calledActorUID)
	}
}

func TestAccountHandler_Delete_MissingUID_400(t *testing.T) {
	svc := &fakeSvc{}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":""}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing_uid") {
		t.Fatalf("body=%s, want missing_uid error code", rec.Body.String())
	}
	if svc.called {
		t.Fatal("service.DeleteAccount should not be called on validation failure")
	}
}

func TestAccountHandler_Delete_InvalidJSON_400(t *testing.T) {
	svc := &fakeSvc{}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`not-json`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.called {
		t.Fatal("service.DeleteAccount should not be called on validation failure")
	}
}

func TestAccountHandler_Delete_Forbidden_403(t *testing.T) {
	svc := &fakeSvc{err: apperrors.Forbidden("not_a_member", "actor has no role on this tenant")}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":"stranger-1"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.calledTenantID != "t1" || svc.calledActorUID != "stranger-1" {
		t.Fatalf("unexpected call args: tenantID=%q actorUID=%q", svc.calledTenantID, svc.calledActorUID)
	}
}

func TestAccountHandler_Delete_NotFound_404(t *testing.T) {
	svc := &fakeSvc{err: apperrors.NotFound("tenant_not_found", "tenant does not exist")}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/missing-tenant/account",
		strings.NewReader(`{"uid":"owner-1"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.calledTenantID != "missing-tenant" {
		t.Fatalf("tenantID=%q, want missing-tenant", svc.calledTenantID)
	}
}

// teardownTenantID is a real UUID because the teardown handler now
// validates its path parameter before it can reach a `WHERE id = ?`
// comparison against a uuid column. "t-1" would be answered 400 and never
// reach the service.
const teardownTenantID = "11111111-1111-1111-1111-111111111111"

type fakePurger struct {
	res *PurgeResult
	err error
	got []string
}

func (f *fakePurger) DeleteAccount(_ context.Context, _, _ string) error { return nil }
func (f *fakePurger) PurgeTenant(_ context.Context, _ string, slugs []string) (*PurgeResult, error) {
	f.got = slugs
	return f.res, f.err
}

func teardownRouter(svc accountDeleter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(svc).RegisterOperator(r.Group("/internal"))
	return r
}

func TestTeardown_SuccessReturnsResult(t *testing.T) {
	f := &fakePurger{res: &PurgeResult{
		TenantID: teardownTenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1"}, StoreSlugs: []string{"the-bondi-store"},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/"+teardownTenantID+"/teardown",
		strings.NewReader(`{"store_slugs":["the-bondi-store"]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"the-bondi-store"}, f.got)
	require.JSONEq(t, `{"data":{"tenant_id":"`+teardownTenantID+`","tenant_name":"The Bondi Store","store_ids":["s-1"],"store_slugs":["the-bondi-store"]}}`, rec.Body.String())
}

func TestTeardown_MismatchIs409WithExpectedSet(t *testing.T) {
	f := &fakePurger{err: &MismatchError{Expected: []string{"a", "b"}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/"+teardownTenantID+"/teardown",
		strings.NewReader(`{"store_slugs":["wrong"]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"confirmation_mismatch","message":"supplied store_slugs do not match the tenant's current stores","expected":["a","b"]}`, rec.Body.String())
}

func TestTeardown_NotFoundIs404(t *testing.T) {
	f := &fakePurger{err: apperrors.NotFound("tenant_not_found", "nope")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/"+teardownTenantID+"/teardown",
		strings.NewReader(`{"store_slugs":[]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// An ABSENT store_slugs must fail. An EMPTY one is a legitimate assertion
// that the tenant has no stores, and only matches a tenant that has none.
// The two are one character apart on the wire and must not collapse.
func TestTeardown_AbsentStoreSlugsIs400_EmptyIsAccepted(t *testing.T) {
	absent := httptest.NewRecorder()
	teardownRouter(&fakePurger{}).ServeHTTP(absent,
		httptest.NewRequest(http.MethodPost, "/internal/tenants/"+teardownTenantID+"/teardown", strings.NewReader(`{}`)))
	require.Equal(t, http.StatusBadRequest, absent.Code)

	f := &fakePurger{res: &PurgeResult{TenantID: teardownTenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	empty := httptest.NewRecorder()
	teardownRouter(f).ServeHTTP(empty,
		httptest.NewRequest(http.MethodPost, "/internal/tenants/"+teardownTenantID+"/teardown", strings.NewReader(`{"store_slugs":[]}`)))
	require.Equal(t, http.StatusOK, empty.Code)
	require.NotNil(t, f.got, "an empty array must reach the service as a non-nil empty slice")
	require.Empty(t, f.got)
}

// A non-UUID id must be answered 400 and must NOT reach the service.
//
// Without this check the id goes straight into PurgeTenant's
// `WHERE id = ?` against a uuid column, Postgres raises a cast error, and
// respondError answers 500 — a malformed request reported as a server
// fault on the one endpoint where "the server broke" and "you sent
// nonsense" must not look the same. marketplace-api parses the id before
// it ever calls here, but this strict internal group is callable
// in-cluster by anything holding the shared secret.
func TestTeardown_NonUUIDIdIs400AndNeverReachesTheService(t *testing.T) {
	f := &fakePurger{res: &PurgeResult{TenantID: "whatever"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/not-a-uuid/teardown",
		strings.NewReader(`{"store_slugs":[]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.JSONEq(t,
		`{"error":"invalid_tenant_id","message":"id must be a UUID","field":"id"}`,
		rec.Body.String())
	require.Nil(t, f.got, "an unparseable id must never reach PurgeTenant — it would destroy a tenant")
}
