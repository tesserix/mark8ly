package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// stubDirRepo records the filter it received and returns a canned result.
type stubDirRepo struct {
	Repository
	got    DirectoryFilter
	result DirectoryResult
	detail *TenantWithStores
	err    error

	byID          *Tenant
	byIDErr       error
	byOwnerEmail  *Tenant
	byOwnerErr    error
	gotOwnerEmail string
}

func (s *stubDirRepo) ListDirectory(_ context.Context, f DirectoryFilter) (DirectoryResult, error) {
	s.got = f
	return s.result, s.err
}

func (s *stubDirRepo) GetWithStores(_ context.Context, _ string) (*TenantWithStores, error) {
	return s.detail, s.err
}

func (s *stubDirRepo) GetByID(_ context.Context, _ string) (*Tenant, error) {
	return s.byID, s.byIDErr
}

func (s *stubDirRepo) GetByOwnerEmail(_ context.Context, email string) (*Tenant, error) {
	s.gotOwnerEmail = email
	return s.byOwnerEmail, s.byOwnerErr
}

func dirRouter(t *testing.T, repo Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(NewService(repo, nil), nil).RegisterDirectory(r.Group(""))
	return r
}

func TestDirectoryList_ParsesFilters(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?q=acme&status=active&page=2&limit=25", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "acme", repo.got.Q)
	require.Equal(t, "active", repo.got.Status)
	require.Equal(t, 2, repo.got.Page)
	require.Equal(t, 25, repo.got.Limit)
}

// A missing parameter takes the default and is never an error; an oversized
// one clamps rather than being refused.
func TestDirectoryList_DefaultsAndClamps(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, DefaultDirectoryPageSize, repo.got.Limit)

	repo2 := &stubDirRepo{}
	rec2 := httptest.NewRecorder()
	dirRouter(t, repo2).ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/tenants?limit=100000", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, MaxDirectoryPageSize, repo2.got.Limit)
}

func TestDirectoryList_GarbageParamsDoNotError(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?limit=abc&page=-4&created_from=notadate", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, DefaultDirectoryPageSize, repo.got.Limit)
	require.Equal(t, 1, repo.got.Page)
}

// TestDirectoryList_ParsesIDs asserts a comma-separated ids query param is
// split and trimmed into DirectoryFilter.IDs (#285's batch lookup).
func TestDirectoryList_ParsesIDs(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?ids=a,b", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"a", "b"}, repo.got.IDs)
}

// TestDirectoryList_AllEmptySegmentsIDsIsAbsent is the handler-level
// counterpart to applyDirectoryFilter's len() guard: "?ids=,," must never
// reach the repository as an empty (non-nil) slice, because an empty slice
// becomes "IN ()" in the query and silently returns zero tenants for the
// live /admin/entities/tenants estate view. A mutation that emits an empty
// slice here instead of leaving IDs unset must fail this test.
func TestDirectoryList_AllEmptySegmentsIDsIsAbsent(t *testing.T) {
	repo := &stubDirRepo{result: DirectoryResult{Tenants: []Tenant{{ID: "t1"}}, Total: 1}}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?ids=,,", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.got.IDs)

	var body struct {
		Data []Tenant `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
}

// TestDirectoryList_BlankIDsIsAbsent asserts a present-but-blank "ids="
// behaves the same as the parameter being absent entirely.
func TestDirectoryList_BlankIDsIsAbsent(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?ids=", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.got.IDs)
}

// TestDirectoryList_WhitespaceIDsAreTrimmed asserts individual ids are
// trimmed of surrounding whitespace before reaching the filter.
func TestDirectoryList_WhitespaceIDsAreTrimmed(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?ids=%20a%20,%20b%20", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"a", "b"}, repo.got.IDs)
}

// TestDirectoryList_NoIDsParamIsAbsent asserts the no-ids-at-all case:
// IDs stays empty and the full directory is returned.
func TestDirectoryList_NoIDsParamIsAbsent(t *testing.T) {
	repo := &stubDirRepo{result: DirectoryResult{Tenants: []Tenant{{ID: "t1"}}, Total: 1}}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.got.IDs)

	var body struct {
		Data []Tenant `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
}

func TestDirectoryList_EmptyIsArrayNotNull(t *testing.T) {
	repo := &stubDirRepo{result: DirectoryResult{Tenants: nil, Total: 0}}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants", nil))

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

// TestGetTenantByOwnerEmail_Hit covers the happy path (#279): a seeded
// tenant's owner email resolves to a 200 with the matching tenant.
func TestGetTenantByOwnerEmail_Hit(t *testing.T) {
	repo := &stubDirRepo{byOwnerEmail: &Tenant{ID: "tenant-123", OwnerEmail: "founder@acme.example"}}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants/by-owner-email?email=founder@acme.example", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "founder@acme.example", repo.gotOwnerEmail)

	var body struct {
		Data Tenant `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "tenant-123", body.Data.ID)
	require.Equal(t, "founder@acme.example", body.Data.OwnerEmail)
}

// TestGetTenantByOwnerEmail_Miss covers no tenant owning the given email:
// the repository's apperrors.NotFound must map to a 404 via respondError.
func TestGetTenantByOwnerEmail_Miss(t *testing.T) {
	repo := &stubDirRepo{byOwnerErr: apperrors.NotFound("tenant_not_found", "no tenant owns that email")}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants/by-owner-email?email=nobody@nowhere.example", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetTenantByOwnerEmail_MissingEmail asserts an absent email query
// param reaches the repository as "" and comes back 404, not a 500 or a
// panic. The 400-for-console-callers contract belongs to marketplace-api,
// not this hop.
func TestGetTenantByOwnerEmail_MissingEmail(t *testing.T) {
	repo := &stubDirRepo{byOwnerErr: apperrors.NotFound("tenant_not_found", "no tenant owns that email")}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants/by-owner-email", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "", repo.gotOwnerEmail)
}

// fullRouter mounts the tenant handler exactly the way cmd/server/main.go
// does: Register on a permissive /internal group and RegisterDirectory on
// a SEPARATE strict /internal group with the same base path. This is the
// real two-group shape from main.go:340-348 (gin 1.12) — verified not to
// panic at router-build time and not to shadow sibling routes. That
// verification is load-bearing (see issue #287, a router-panic-at-startup
// class of bug), so it is asserted here rather than only in prose.
func fullRouter(t *testing.T, repo Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(NewService(repo, nil), nil)

	public := r.Group("")
	internal := r.Group("/internal")
	h.Register(public, internal)

	tenantDirectory := r.Group("/internal")
	h.RegisterDirectory(tenantDirectory)

	return r
}

// TestRoute_ByOwnerEmailIsStaticSiblingAndDoesNotShadow locks in that
// adding the static /by-owner-email route next to Register's /:id and
// RegisterDirectory's /:id/detail does not panic the router and does not
// shadow either sibling. A future route edit that reintroduces a
// conflicting wildcard/static pair at this path would fail this test
// before it could panic the service at startup.
func TestRoute_ByOwnerEmailIsStaticSiblingAndDoesNotShadow(t *testing.T) {
	repo := &stubDirRepo{
		byID:         &Tenant{ID: "id-tenant", OwnerEmail: "id@acme.example"},
		detail:       &TenantWithStores{Tenant: Tenant{ID: "detail-tenant"}},
		byOwnerEmail: &Tenant{ID: "email-tenant", OwnerEmail: "owner@acme.example"},
	}

	require.NotPanics(t, func() {
		fullRouter(t, repo)
	})

	r := fullRouter(t, repo)

	// New static route resolves to its own handler, not the :id wildcard.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/tenants/by-owner-email?email=owner@acme.example", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var byEmail struct {
		Data Tenant `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &byEmail))
	require.Equal(t, "email-tenant", byEmail.Data.ID)

	// /internal/tenants/:id (from Register's permissive group) still resolves.
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/internal/tenants/some-id", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	var byID struct {
		Data Tenant `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &byID))
	require.Equal(t, "id-tenant", byID.Data.ID)

	// /internal/tenants/:id/detail (from RegisterDirectory's strict group)
	// still resolves and is not shadowed by the new static route.
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/internal/tenants/some-id/detail", nil))
	require.Equal(t, http.StatusOK, rec3.Code)
	var detail struct {
		Data TenantWithStores `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &detail))
	require.Equal(t, "detail-tenant", detail.Data.ID)
}
