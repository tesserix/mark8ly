package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubDirRepo records the filter it received and returns a canned result.
type stubDirRepo struct {
	Repository
	got    DirectoryFilter
	result DirectoryResult
	detail *TenantWithStores
	err    error
}

func (s *stubDirRepo) ListDirectory(_ context.Context, f DirectoryFilter) (DirectoryResult, error) {
	s.got = f
	if s.result.Tenants == nil {
		s.result.Tenants = []Tenant{}
	}
	return s.result, s.err
}

func (s *stubDirRepo) GetWithStores(_ context.Context, _ string) (*TenantWithStores, error) {
	return s.detail, s.err
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
