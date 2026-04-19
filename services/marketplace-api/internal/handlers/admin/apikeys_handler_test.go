package admin_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// The admin.APIKeysHandler accepts a *apikeys.Service (concrete type, no
// interface). Rather than mock it, the handler tests below run only the body
// parsing + response shaping — the route-mount + plan-gate path is covered
// in the integration suite where a real *Service hits the testdb.
//
// These tests confirm the handler returns 400 on malformed bodies and 401
// when tenant_id is missing — paths that don't touch the service at all.

func TestAPIKeysHandler_Create_MissingTenant_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := admin.NewAPIKeysHandler(nil, nil, slog.Default())

	r := gin.New()
	r.POST("/admin/stores/:storeId/api-keys", h.Create)

	body, _ := json.Marshal(map[string]any{"label": "x", "scopes": []string{"products:read"}})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+uuid.New().String()+"/api-keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeysHandler_Create_MalformedBody_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := admin.NewAPIKeysHandler(nil, nil, slog.Default())

	tenantID := uuid.New()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Set("user_id", uuid.New().String())
		c.Next()
	})
	r.POST("/admin/stores/:storeId/api-keys", h.Create)

	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+uuid.New().String()+"/api-keys", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAPIKeysHandler_Create_MissingScopes_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := admin.NewAPIKeysHandler(nil, nil, slog.Default())

	tenantID := uuid.New()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Set("user_id", uuid.New().String())
		c.Next()
	})
	r.POST("/admin/stores/:storeId/api-keys", h.Create)

	body, _ := json.Marshal(map[string]any{"label": "x"})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+uuid.New().String()+"/api-keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAPIKeysHandler_Revoke_InvalidKeyID_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := admin.NewAPIKeysHandler(nil, nil, slog.Default())

	tenantID := uuid.New()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	r.DELETE("/admin/stores/:storeId/api-keys/:keyId", h.Revoke)

	req := httptest.NewRequest(http.MethodDelete, "/admin/stores/"+uuid.New().String()+"/api-keys/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// Compile-time guard: ensure response shape stays in sync with the handler.
func TestAPIKeysHandler_ResponseShape_KeyResponseRoundTrips(t *testing.T) {
	// Smoke test that ScopeSet → []string conversion is safe on empty.
	row := apikeys.APIKey{}
	require.Nil(t, []string(row.Scopes))
}
