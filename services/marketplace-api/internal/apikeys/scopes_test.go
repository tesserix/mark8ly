package apikeys_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
)

func TestAllScopes_ContainsExpectedSetV1(t *testing.T) {
	all := apikeys.AllScopes()
	for _, want := range []string{
		"products:read", "products:write",
		"orders:read", "orders:write",
		"customers:read", "customers:write",
		"categories:read", "categories:write",
		"coupons:read", "coupons:write",
	} {
		require.Contains(t, all, apikeys.Scope(want))
	}
	require.NotContains(t, all, apikeys.Scope("admin:all"))
	require.NotContains(t, all, apikeys.Scope("tenant:admin"))
}

func TestValidateScopes_AcceptsKnownSet(t *testing.T) {
	require.NoError(t, apikeys.ValidateScopes([]string{"products:read", "orders:read"}))
	require.NoError(t, apikeys.ValidateScopes(nil))
}

func TestValidateScopes_RejectsUnknown(t *testing.T) {
	require.Error(t, apikeys.ValidateScopes([]string{"products:read", "delete:everything"}))
	require.Error(t, apikeys.ValidateScopes([]string{"admin:all"}))
}

func TestIsReadOnlyScope(t *testing.T) {
	require.True(t, apikeys.IsReadOnlyScope("products:read"))
	require.False(t, apikeys.IsReadOnlyScope("products:write"))
	require.False(t, apikeys.IsReadOnlyScope("unknown"))
}

func TestAllReadOnly(t *testing.T) {
	require.True(t, apikeys.AllReadOnly([]string{"products:read", "orders:read"}))
	require.False(t, apikeys.AllReadOnly([]string{"products:read", "orders:write"}))
	require.True(t, apikeys.AllReadOnly(nil))
}

func TestRequireScope_Allows_WhenScopePresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("api_key_scopes", []string{"products:read", "orders:read"})
		c.Next()
	})
	r.GET("/v1/products", apikeys.RequireScope(apikeys.ScopeProductsRead), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/products", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireScope_403_WhenScopeMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("api_key_scopes", []string{"products:read"})
		c.Next()
	})
	r.POST("/v1/products", apikeys.RequireScope(apikeys.ScopeProductsWrite), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/products", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireScope_401_WhenContextMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/products", apikeys.RequireScope(apikeys.ScopeProductsRead), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/products", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
