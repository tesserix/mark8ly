//go:build integration

package vendor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandler_EnsureSelfVendor_Creates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	tenantID := "55555555-5555-5555-5555-555555555555"
	body, _ := json.Marshal(map[string]string{"name": "Acme", "slug": "acme-" + tenantID})
	req := httptest.NewRequest(http.MethodPost,
		"/internal/tenants/"+tenantID+"/ensure-self-vendor",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data Vendor `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "Acme", resp.Data.Name)
	require.True(t, resp.Data.IsSelf)
}

func TestHandler_EnsureSelfVendor_Idempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	tenantID := "66666666-6666-6666-6666-666666666666"
	slug := "acme-" + tenantID
	body, _ := json.Marshal(map[string]string{"name": "Acme", "slug": slug})

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost,
			"/internal/tenants/"+tenantID+"/ensure-self-vendor",
			bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	first := do()
	require.Equal(t, http.StatusOK, first.Code)
	second := do()
	require.Equal(t, http.StatusOK, second.Code)

	var f, s struct{ Data Vendor `json:"data"` }
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &f))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &s))
	require.Equal(t, f.Data.ID, s.Data.ID, "idempotent ensure returns the same vendor")
}

func TestHandler_EnsureSelfVendor_BadInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	req := httptest.NewRequest(http.MethodPost,
		"/internal/tenants/77777777-7777-7777-7777-777777777777/ensure-self-vendor",
		bytes.NewReader([]byte(`{"name": "", "slug": "x"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "validation_failed", resp.Error)
}

func TestHandler_GetSelfVendor_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/tenants/88888888-8888-8888-8888-888888888888/self-vendor", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_GetByID_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	req := httptest.NewRequest(http.MethodGet,
		"/internal/vendors/99999999-9999-9999-9999-999999999999", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_UpdateSelfVendor_OverwritesNameAndSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	tenantID := "cccccccc-cccc-cccc-cccc-cccccccccccc"

	// Ensure first (creates placeholder), then PATCH.
	_, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Merchant", "placeholder-"+tenantID)
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]string{"name": "Acme", "slug": "acme-" + tenantID})
	req := httptest.NewRequest(http.MethodPatch,
		"/internal/tenants/"+tenantID+"/self-vendor",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data Vendor `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "Acme", resp.Data.Name)
	require.Equal(t, "acme-"+tenantID, resp.Data.Slug)
}

func TestHandler_UpdateSelfVendor_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	body, _ := json.Marshal(map[string]string{"name": "Acme", "slug": "acme"})
	req := httptest.NewRequest(http.MethodPatch,
		"/internal/tenants/dddddddd-dddd-dddd-dddd-dddddddddddd/self-vendor",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
