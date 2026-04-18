package readonly_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/readonly"
)

// makeRouter registers a test endpoint at the given pattern, wires
// RequireActive with the given status, and returns the engine so the caller
// can do an httptest round-trip.
func makeRouter(status subscription.SubscriptionStatus, method, pattern string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("subscription_status", status)
		c.Next()
	})
	r.Use(readonly.RequireActive(readonly.Config{}))
	r.Handle(method, pattern, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func doReq(r *gin.Engine, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireActive_BlocksExpiredOnNonAllowlistedRoute(t *testing.T) {
	r := makeRouter(subscription.StatusExpired, http.MethodGet, "/admin/stores/:storeId/products")
	// But GET /admin/** is always allowed — use POST instead for the block case.
	r2 := makeRouter(subscription.StatusExpired, http.MethodPost, "/admin/stores/:storeId/products")
	w := doReq(r2, http.MethodPost, "/admin/stores/s1/products")
	require.Equal(t, http.StatusPaymentRequired, w.Code)
	require.Contains(t, w.Body.String(), "subscription_inactive")
	_ = r
}

func TestRequireActive_AllowsBillingPathEvenWhenExpired(t *testing.T) {
	r := makeRouter(subscription.StatusExpired, http.MethodPost, "/admin/stores/:storeId/subscription/checkout")
	w := doReq(r, http.MethodPost, "/admin/stores/s1/subscription/checkout")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireActive_AllowsOrderExportEvenWhenStoreClosed(t *testing.T) {
	r := makeRouter(subscription.StatusStoreClosed, http.MethodGet, "/admin/stores/:storeId/orders/export/*path")
	w := doReq(r, http.MethodGet, "/admin/stores/s1/orders/export/csv")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireActive_AllowsGetAdminReadsEvenWhenExpired(t *testing.T) {
	// GET /admin/** is view-only — always allowed.
	r := makeRouter(subscription.StatusExpired, http.MethodGet, "/admin/stores/:storeId/products")
	w := doReq(r, http.MethodGet, "/admin/stores/s1/products")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireActive_DoesNotBlockPaymentActionRequired(t *testing.T) {
	// Council finding #3: PAR merchants retain full admin access.
	r := makeRouter(subscription.StatusPaymentActionRequired, http.MethodPut, "/admin/stores/:storeId/products/:id")
	w := doReq(r, http.MethodPut, "/admin/stores/s1/products/42")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireActive_DoesNotBlockActive(t *testing.T) {
	r := makeRouter(subscription.StatusActive, http.MethodPut, "/admin/stores/:storeId/products/:id")
	w := doReq(r, http.MethodPut, "/admin/stores/s1/products/42")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequireActive_BlocksPendingHardDelete(t *testing.T) {
	r := makeRouter(subscription.StatusPendingHardDelete, http.MethodPost, "/admin/stores/:storeId/products")
	w := doReq(r, http.MethodPost, "/admin/stores/s1/products")
	require.Equal(t, http.StatusPaymentRequired, w.Code)
}

func TestRequireActive_NoStatusOnContext_FallsThrough(t *testing.T) {
	// If StoreMiddleware never ran, RequireActive must not block.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(readonly.RequireActive(readonly.Config{}))
	r.POST("/admin/stores/:storeId/products", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := doReq(r, http.MethodPost, "/admin/stores/s1/products")
	require.Equal(t, http.StatusOK, w.Code)
}
