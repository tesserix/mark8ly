//go:build integration

package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/delivery"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestIntegration_DeliverySettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, "store_delivery_options", "store_couriers", "stores")
	storeID, tid := seedStoreRow(t, db, "")
	repo := stores.NewRepository(db)
	h := admin.NewDeliverySettingsHandler(delivery.NewService(db), nil)

	e := gin.New()
	g := e.Group("/s", func(c *gin.Context) {
		s, err := repo.GetByIDForTenant(c.Request.Context(), storeID, tid)
		if err != nil {
			t.Fatal(err)
		}
		c.Set("store", s)
	})
	g.GET("", h.Get)
	g.PUT("/options", h.PutOptions)
	g.POST("/couriers", h.CreateCourier)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		return w
	}

	if w := do("GET", "/s", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"mode":"carrier"`) {
		t.Fatalf("GET: %d %s", w.Code, w.Body)
	}
	w := do("PUT", "/s/options", `{"options":[{"name":"Std","price":"5.00","enabled":true}],"free_shipping_min":"100"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Std"`) || !strings.Contains(w.Body.String(), `"free_shipping_min":"100"`) {
		t.Fatalf("PUT: %d %s", w.Code, w.Body)
	}
	w = do("POST", "/s/couriers", `{"name":"C","tracking_url_template":"http://x/{number}"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "validation_failed") {
		t.Fatalf("courier: %d %s", w.Code, w.Body)
	}
}
