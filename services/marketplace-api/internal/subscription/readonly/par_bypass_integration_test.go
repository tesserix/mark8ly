//go:build integration

package readonly_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/readonly"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestIntegration_PARBypass verifies that a store in payment_action_required
// retains full admin access (Council finding #3). The test exercises the
// complete LoadStatus + RequireActive middleware chain against a real database
// row — not a mocked context value.
func TestIntegration_PARBypass(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_par",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Simulate the GIP/IstioAuth middleware setting tenant_id on the context.
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	r.Use(readonly.LoadStatus(readonly.StatusLoaderConfig{
		DB:     db,
		Repo:   repo,
		Logger: slog.Default(),
	}))
	r.Use(readonly.RequireActive(readonly.Config{}))

	// A mutation route that would be blocked for truly read-only statuses.
	r.POST("/admin/stores/:storeId/products", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/products", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"payment_action_required must NOT block admin routes (Council finding #3)")
}

// TestIntegration_ExpiredBlocked is a sanity check that the same middleware
// chain DOES block when the status is truly read-only (expired).
func TestIntegration_ExpiredBlocked(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_exp",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusExpired,
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	r.Use(readonly.LoadStatus(readonly.StatusLoaderConfig{
		DB:     db,
		Repo:   repo,
		Logger: slog.Default(),
	}))
	r.Use(readonly.RequireActive(readonly.Config{}))
	r.POST("/admin/stores/:storeId/products", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/products", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusPaymentRequired, w.Code)
	require.Contains(t, w.Body.String(), "subscription_inactive")
	require.Contains(t, w.Body.String(), "expired")
}
