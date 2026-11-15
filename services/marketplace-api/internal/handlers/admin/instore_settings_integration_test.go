//go:build integration

package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestIntegration_InStoreSettings_SavesAndSurvivesSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, "stores")
	storeID, tid := seedStoreRow(t, db, "")
	fga := authz.NewFakeClient()
	fga.Grant("u-1", authz.RoleAdmin, tid)
	h := admin.NewInStoreHandler(db, nil, fga, nil)
	repo := stores.NewRepository(db)

	e := gin.New()
	g := e.Group("/admin/stores/:storeId", func(c *gin.Context) {
		s, err := repo.GetByIDForTenant(c.Request.Context(), storeID, tid)
		if err != nil {
			t.Fatalf("load store: %v", err)
		}
		c.Set("user_id", "u-1")
		c.Set("tenant_id", tid)
		c.Set("store", s)
	})
	g.PUT("/in-store/settings", h.UpdateSettings)
	g.GET("/in-store/settings", h.Settings)

	req := httptest.NewRequest(http.MethodPut, "/admin/stores/"+storeID+"/in-store/settings",
		strings.NewReader(`{"cashier_discounts_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"cashier_discounts_enabled":true}` {
		t.Fatalf("PUT: %d %s", w.Code, w.Body)
	}

	// A platform-api sync rewrites the mirrored columns; the flag is ours.
	s, err := repo.GetByIDForTenant(t.Context(), storeID, tid)
	if err != nil {
		t.Fatal(err)
	}
	s.Name = "Renamed"
	s.CashierDiscountsEnabled = false
	if err := repo.Upsert(t.Context(), s); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/stores/"+storeID+"/in-store/settings", nil))
	if strings.TrimSpace(w.Body.String()) != `{"cashier_discounts_enabled":true}` {
		t.Errorf("after sync: %s", w.Body)
	}
}
