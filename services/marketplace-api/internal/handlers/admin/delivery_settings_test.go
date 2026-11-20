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
	for name, body := range map[string]string{
		"no options":    `{"free_shipping_min":null}`,
		"null options":  `{"options":null,"free_shipping_min":null}`,
		"no price":      `{"options":[{"name":"A","enabled":true}],"free_shipping_min":null}`,
		"no free min":   `{"options":[]}`,
		"free min huge": `{"options":[],"free_shipping_min":"1e20"}`,
		"price 100000":  `{"options":[{"name":"A","price":"100000","enabled":true}],"free_shipping_min":null}`,
	} {
		if w := do("PUT", "/s/options", body); w.Code != 400 || !strings.Contains(w.Body.String(), "validation_failed") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// Rejected requests did not wipe the earlier save; explicit empty/null does.
	if w := do("GET", "/s", ""); !strings.Contains(w.Body.String(), `"name":"Std"`) {
		t.Fatalf("wiped: %s", w.Body)
	}
	if w := do("PUT", "/s/options", `{"options":[],"free_shipping_min":null}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"free_shipping_min":null`) {
		t.Fatalf("clear: %d %s", w.Code, w.Body)
	}
	w = do("POST", "/s/couriers", `{"name":"C","tracking_url_template":"http://x/{number}"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "validation_failed") {
		t.Fatalf("courier: %d %s", w.Code, w.Body)
	}
}
