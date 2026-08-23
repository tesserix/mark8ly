package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// stubDirectory records params and returns canned results.
type stubDirectory struct {
	gotParams tenantdirectory.ListParams
	list      *tenantdirectory.ListResult
	detail    *tenantdirectory.TenantDetail
	err       error
}

func (s *stubDirectory) List(_ context.Context, p tenantdirectory.ListParams) (*tenantdirectory.ListResult, error) {
	s.gotParams = p
	if s.err != nil {
		return nil, s.err
	}
	if s.list == nil {
		s.list = &tenantdirectory.ListResult{Tenants: []tenantdirectory.Tenant{}}
	}
	return s.list, nil
}

func (s *stubDirectory) Get(_ context.Context, _ string) (*tenantdirectory.TenantDetail, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.detail, nil
}

func tenantsRouter(t *testing.T, dir platformadmin.TenantDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewEntitiesTenantsHandler(dir, nil).Register(r.Group(""))
	return r
}

// THE test. Real handler output compared to the committed contract.
func TestEntitiesTenantsMatchesContract(t *testing.T) {
	dir := &stubDirectory{list: &tenantdirectory.ListResult{
		Total: 2, Page: 1, Limit: 50,
		Tenants: []tenantdirectory.Tenant{
			{
				ID: "3f2504e0-4f89-11d3-9a0c-0305e82c3301", Name: "Acme Trading",
				OwnerEmail: "founder@acme.example", Status: "active",
				CreatedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
			},
			{
				ID: "3f2504e0-4f89-11d3-9a0c-0305e82c3302", Name: "Beta Goods",
				OwnerEmail: "owner@beta.example", Status: "suspended",
				CreatedAt: time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC),
			},
		},
	}}

	rec := httptest.NewRecorder()
	tenantsRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/entities_tenants_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

func TestEntitiesTenantsEmptyIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	tenantsRouter(t, &stubDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

func TestEntitiesTenantsPassesFilters(t *testing.T) {
	dir := &stubDirectory{}
	rec := httptest.NewRecorder()
	tenantsRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants?q=acme&status=active&limit=25&page=2", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "acme", dir.gotParams.Q)
	require.Equal(t, "active", dir.gotParams.Status)
	require.Equal(t, 25, dir.gotParams.Limit)
	require.Equal(t, 2, dir.gotParams.Page)
}

// An unreachable upstream must NOT look like an empty directory. A 200 with
// no tenants would have a console operator conclude none exist.
func TestEntitiesTenantsUpstreamDownIs503(t *testing.T) {
	rec := httptest.NewRecorder()
	tenantsRouter(t, &stubDirectory{err: tenantdirectory.ErrUnavailable}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/entities/tenants", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
}

func TestEntitiesTenantDetailNotFoundIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	tenantsRouter(t, &stubDirectory{err: tenantdirectory.ErrNotFound}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/entities/tenants/missing", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "not_found", errorCode(t, rec))
}

func TestEntitiesTenantDetailReturnsRollup(t *testing.T) {
	dir := &stubDirectory{detail: &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{
			ID: "t1", Name: "Acme", OwnerEmail: "a@example.com", Status: "active",
			CreatedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		},
		Stores: []tenantdirectory.StoreSummary{
			{ID: "s1", Slug: "acme", Name: "Acme Store", Status: "active"},
		},
	}}

	rec := httptest.NewRecorder()
	tenantsRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants/t1", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			ID         string `json:"id"`
			StoreCount int    `json:"store_count"`
			Stores     []struct {
				Slug string `json:"slug"`
			} `json:"stores"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "t1", body.Data.ID)
	require.Equal(t, 1, body.Data.StoreCount)
	require.Len(t, body.Data.Stores, 1)
	require.Equal(t, "acme", body.Data.Stores[0].Slug)
}
