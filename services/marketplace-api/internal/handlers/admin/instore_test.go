package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

type fakeRecorder struct {
	got    instore.SaleInput
	reused bool
	err    error
}

func (f *fakeRecorder) RecordSale(_ context.Context, in instore.SaleInput) (*order.CreateResult, error) {
	f.got = in
	if f.err != nil {
		return nil, f.err
	}
	return &order.CreateResult{Order: &order.Order{ID: uuid.New(), Channel: order.ChannelInStore}, Reused: f.reused}, nil
}

const tenantID = "11111111-1111-1111-1111-111111111111"

func inStoreEngine(t *testing.T, rec *fakeRecorder, role authz.Role, store *stores.Store) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	fga := authz.NewFakeClient()
	fga.Grant("u-1", role, tenantID)
	h := admin.NewInStoreHandler(nil, rec, fga, nil)
	e := gin.New()
	g := e.Group("/admin/stores/:storeId", func(c *gin.Context) {
		c.Set("user_id", "u-1")
		c.Set("tenant_id", tenantID)
		c.Set("store", store)
	})
	g.POST("/in-store/sales", h.RecordSale)
	return e
}

func postSale(e *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+uuid.NewString()+"/in-store/sales", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

const saleBody = `{"idempotency_key":"k1","lines":[{"variant_id":"v1","quantity":1}],"payment_method":"card_external"}`

func TestInStoreSale_Created201(t *testing.T) {
	rec := &fakeRecorder{}
	w := postSale(inStoreEngine(t, rec, authz.RoleCashier, &stores.Store{}), saleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	if rec.got.SoldByUserID != "u-1" || !rec.got.Fulfilled {
		t.Errorf("sold by %q fulfilled %v, want u-1 and true by default", rec.got.SoldByUserID, rec.got.Fulfilled)
	}
}

func TestInStoreSale_Replay200(t *testing.T) {
	w := postSale(inStoreEngine(t, &fakeRecorder{reused: true}, authz.RoleCashier, &stores.Store{}), saleBody)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestInStoreSale_OutOfStock409NamesVariant(t *testing.T) {
	rec := &fakeRecorder{err: checkoutcore.OutOfStockError{VariantID: "v1"}}
	w := postSale(inStoreEngine(t, rec, authz.RoleCashier, &stores.Store{}), saleBody)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "out_of_stock" || body["variant_id"] != "v1" {
		t.Errorf("body = %v", body)
	}
}

func TestInStoreSale_CashierDiscountOnlyWhenEnabled(t *testing.T) {
	rec := &fakeRecorder{}
	postSale(inStoreEngine(t, rec, authz.RoleCashier, &stores.Store{}), saleBody)
	if rec.got.CanDiscount {
		t.Error("cashier may not discount unless the store allows it")
	}

	rec = &fakeRecorder{}
	postSale(inStoreEngine(t, rec, authz.RoleCashier, &stores.Store{CashierDiscountsEnabled: true}), saleBody)
	if !rec.got.CanDiscount {
		t.Error("store allows cashier discounts")
	}

	rec = &fakeRecorder{}
	postSale(inStoreEngine(t, rec, authz.RoleStaff, &stores.Store{}), saleBody)
	if !rec.got.CanDiscount {
		t.Error("staff may always discount")
	}
}
