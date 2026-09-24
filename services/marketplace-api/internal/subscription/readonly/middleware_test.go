package readonly_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/readonly"
)

// basePrefix is where production mounts the admin group:
//
//	admin.RegisterAdmin(r.Group("/api/v1"), adminDeps)
//
// so c.FullPath() is "/api/v1/admin/...". These tests used to mount at
// "/admin/..." with no base group, which made every assertion here describe
// a path shape production never produces — and hid a total failure of the
// allowlist, recovery routes included. Keep the base group.
const basePrefix = "/api/v1"

// makeRouter registers a test endpoint at the given pattern UNDER the base
// prefix, wires RequireActive with the given status, and returns the engine
// so the caller can do an httptest round-trip.
func makeRouter(status subscription.SubscriptionStatus, method, pattern string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("subscription_status", status)
		c.Next()
	})
	r.Use(readonly.RequireActive(readonly.Config{}))
	r.Group(basePrefix).Handle(method, pattern, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// doReq targets the route as a client would, i.e. including the base prefix.
func doReq(r *gin.Engine, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, basePrefix+target, nil)
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
	r.Group(basePrefix).POST("/admin/stores/:storeId/products", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := doReq(r, http.MethodPost, "/admin/stores/s1/products")
	require.Equal(t, http.StatusOK, w.Code)
}

// Every route DefaultAllowlist names must actually be reachable in a
// read-only state, at the path shape production serves.
//
// This is the test that was missing. The package doc promises "Billing +
// subscription + order-export + auth routes are always allowed so merchants
// can recover", and not one of them was: the patterns are written
// "/admin/..." while FullPath() is "/api/v1/admin/...", so a merchant whose
// trial lapsed could not re-subscribe, export their orders, or fix a tax ID.
// The old tests mounted without the base group and so asserted nothing about
// the running service.
func TestRequireActive_RecoveryRoutesReachableInEveryReadOnlyState(t *testing.T) {
	routes := []struct {
		name    string
		method  string
		pattern string
		target  string
	}{
		{"subscription checkout", http.MethodPost, "/admin/stores/:storeId/subscription/*path", "/admin/stores/s1/subscription/checkout"},
		{"billing portal", http.MethodPost, "/admin/stores/:storeId/billing/*path", "/admin/stores/s1/billing/portal"},
		{"order export", http.MethodGet, "/admin/stores/:storeId/orders/export/*path", "/admin/stores/s1/orders/export/csv"},
		{"auth", http.MethodPost, "/admin/auth/*path", "/admin/auth/refresh"},
		{"migration fast path", http.MethodGet, "/admin/stores/:storeId/migration-fast-path/*path", "/admin/stores/s1/migration-fast-path/status"},
		{"complete payment action", http.MethodGet, "/admin/stores/:storeId/subscription/complete-action", "/admin/stores/s1/subscription/complete-action"},
		{"tax id fix", http.MethodPost, "/admin/stores/:storeId/tax/*path", "/admin/stores/s1/tax/id"},
	}
	states := []subscription.SubscriptionStatus{
		subscription.StatusExpired,
		subscription.StatusStoreClosed,
		subscription.StatusPendingHardDelete,
	}

	for _, state := range states {
		for _, rt := range routes {
			t.Run(string(state)+"/"+rt.name, func(t *testing.T) {
				r := makeRouter(state, rt.method, rt.pattern)
				w := doReq(r, rt.method, rt.target)
				require.Equal(t, http.StatusOK, w.Code,
					"a merchant in %s cannot reach %s — this is the route that lets them recover",
					state, rt.target)
			})
		}
	}
}

// The view-only rule must hold at the real path shape, for every read-only
// state: a merchant who stops paying keeps READ access to their own data.
func TestRequireActive_ViewOnlyGETsSurviveEveryReadOnlyState(t *testing.T) {
	for _, state := range []subscription.SubscriptionStatus{
		subscription.StatusExpired,
		subscription.StatusStoreClosed,
		subscription.StatusPendingHardDelete,
	} {
		for _, path := range []string{"products", "orders", "customers", "reviews"} {
			t.Run(string(state)+"/"+path, func(t *testing.T) {
				r := makeRouter(state, http.MethodGet, "/admin/stores/:storeId/"+path)
				w := doReq(r, http.MethodGet, "/admin/stores/s1/"+path)
				require.Equal(t, http.StatusOK, w.Code)
			})
		}
	}
}

// Writes must STILL be blocked. The fix widens what matches; it must not
// turn the gate off.
func TestRequireActive_StillBlocksWritesAtTheRealPathShape(t *testing.T) {
	for _, tc := range []struct{ method, pattern, target string }{
		{http.MethodPost, "/admin/stores/:storeId/products", "/admin/stores/s1/products"},
		{http.MethodPut, "/admin/stores/:storeId/products/:id", "/admin/stores/s1/products/42"},
		{http.MethodDelete, "/admin/stores/:storeId/products/:id", "/admin/stores/s1/products/42"},
		{http.MethodPost, "/admin/stores/:storeId/discounts", "/admin/stores/s1/discounts"},
	} {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			r := makeRouter(subscription.StatusStoreClosed, tc.method, tc.pattern)
			w := doReq(r, tc.method, tc.target)
			require.Equal(t, http.StatusPaymentRequired, w.Code)
		})
	}
}

// A route outside any admin group is not this middleware's business and must
// not be granted access by the prefix-agnostic match. "/store-admin/" in
// particular must NOT be treated as an admin route — the leading slash in the
// "/admin/" marker is what keeps that true.
func TestRequireActive_NonAdminRoutesAreNotSweptIn(t *testing.T) {
	for _, pattern := range []string{
		"/storefront/stores/:storeSlug/products",
		"/store-admin/stores/:storeId/products",
	} {
		t.Run(pattern, func(t *testing.T) {
			r := makeRouter(subscription.StatusStoreClosed, http.MethodPost, pattern)
			target := strings.Replace(pattern, ":storeSlug", "s1", 1)
			target = strings.Replace(target, ":storeId", "s1", 1)
			w := doReq(r, http.MethodPost, target)
			require.Equal(t, http.StatusPaymentRequired, w.Code)
		})
	}
}

// The 402 body must carry a human-readable `message`, because that is the
// field every admin client reads. Its absence is why the UI logged
// "402: unknown error" and showed a generic error boundary instead of
// saying the trial had ended.
func TestRequireActive_402BodyExplainsItself(t *testing.T) {
	for _, tc := range []struct {
		status subscription.SubscriptionStatus
		expect string
	}{
		{subscription.StatusExpired, "trial has ended"},
		{subscription.StatusStoreClosed, "store is closed"},
		{subscription.StatusPendingHardDelete, "scheduled for deletion"},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			r := makeRouter(tc.status, http.MethodPost, "/admin/stores/:storeId/products")
			w := doReq(r, http.MethodPost, "/admin/stores/s1/products")
			require.Equal(t, http.StatusPaymentRequired, w.Code)

			var body struct {
				Error   string `json:"error"`
				Status  string `json:"status"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "subscription_inactive", body.Error)
			require.Equal(t, string(tc.status), body.Status)
			require.Contains(t, body.Message, tc.expect,
				"the message field is what admin clients render; "+
					"without it they show 'unknown error'")
		})
	}
}
