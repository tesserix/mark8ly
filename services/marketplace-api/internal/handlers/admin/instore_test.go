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
	"github.com/shopspring/decimal"

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
	quote  *instore.Quote
}

func (f *fakeRecorder) Quote(_ context.Context, in instore.SaleInput) (*instore.Quote, error) {
	f.got = in
	if f.err != nil {
		return nil, f.err
	}
	return f.quote, nil
}

func (f *fakeRecorder) RecordSale(_ context.Context, in instore.SaleInput) (*order.CreateResult, error) {
	f.got = in
	if f.err != nil {
		return nil, f.err
	}
	return &order.CreateResult{Order: &order.Order{ID: uuid.New(), Channel: order.ChannelInStore}, Reused: f.reused}, nil
}

const tenantID = "11111111-1111-1111-1111-111111111111"

func inStoreEngine(t *testing.T, rec *fakeRecorder, role authz.Role, store *stores.Store, receipts ...admin.InStoreReceipts) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	fga := authz.NewFakeClient()
	fga.Grant("u-1", role, tenantID)
	h := admin.NewInStoreHandler(nil, rec, fga, nil)
	if len(receipts) > 0 {
		h = h.WithReceipts(receipts[0])
	}
	e := gin.New()
	g := e.Group("/admin/stores/:storeId", func(c *gin.Context) {
		c.Set("user_id", "u-1")
		c.Set("tenant_id", tenantID)
		c.Set("store", store)
	})
	g.POST("/in-store/sales", h.RecordSale)
	g.POST("/in-store/quote", h.Quote)
	g.POST("/in-store/sales/:id/receipt", h.Receipt)
	return e
}

func postSale(e *gin.Engine, body string) *httptest.ResponseRecorder {
	return post(e, "sales", body)
}

func post(e *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+uuid.NewString()+"/in-store/"+path, strings.NewReader(body))
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

func TestInStoreQuote_200(t *testing.T) {
	rec := &fakeRecorder{quote: &instore.Quote{
		Subtotal: decimal.RequireFromString("40.00"), TaxTotal: decimal.RequireFromString("4.00"),
		GrandTotal: decimal.RequireFromString("44.00"), CurrencyCode: "AUD",
	}}
	w := post(inStoreEngine(t, rec, authz.RoleCashier, &stores.Store{}), "quote",
		`{"lines":[{"variant_id":"v1","quantity":2}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["grand_total"] != "44.00" || body["tax_total"] != "4.00" || body["currency_code"] != "AUD" {
		t.Errorf("body = %v", body)
	}
}

func TestInStoreSale_TotalChanged409(t *testing.T) {
	rec := &fakeRecorder{err: &instore.TotalChangedError{
		Expected: decimal.RequireFromString("40.00"), Actual: decimal.RequireFromString("46.00"),
	}}
	w := postSale(inStoreEngine(t, rec, authz.RoleCashier, &stores.Store{}),
		`{"idempotency_key":"k1","lines":[{"variant_id":"v1","quantity":1}],"payment_method":"card_external","expected_total":"40.00"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "total_changed" || body["grand_total"] != "46.00" {
		t.Errorf("body = %v", body)
	}
	if rec.got.ExpectedTotal == nil || rec.got.ExpectedTotal.String() != "40" {
		t.Errorf("expected_total not passed through: %v", rec.got.ExpectedTotal)
	}
}

type fakeReceipts struct {
	soldBy string
	sent   string // email passed to Send
	issued bool
}

func (f *fakeReceipts) SoldBy(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.soldBy, nil
}
func (f *fakeReceipts) Issue(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	f.issued = true
	return "https://s.mark8ly.com/r/tok", nil
}
func (f *fakeReceipts) Send(_ context.Context, _, _ uuid.UUID, email string) (string, error) {
	f.sent = email
	return "https://s.mark8ly.com/r/tok", nil
}

func postReceipt(e *gin.Engine, body string) *httptest.ResponseRecorder {
	return post(e, "sales/"+uuid.NewString()+"/receipt", body)
}

func TestReceiptLink_CashierOwnSalesOnly(t *testing.T) {
	r := &fakeReceipts{soldBy: "someone-else"}
	e := inStoreEngine(t, &fakeRecorder{}, authz.RoleCashier, &stores.Store{}, r)
	if w := postReceipt(e, `{"send":false}`); w.Code != http.StatusNotFound || r.issued {
		t.Errorf("cashier on another's sale: %d issued=%v", w.Code, r.issued)
	}

	r = &fakeReceipts{soldBy: "u-1"}
	e = inStoreEngine(t, &fakeRecorder{}, authz.RoleCashier, &stores.Store{}, r)
	w := postReceipt(e, `{"send":false}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"url":"https://s.mark8ly.com/r/tok"}` {
		t.Errorf("cashier own sale: %d %s", w.Code, w.Body)
	}

	r = &fakeReceipts{soldBy: "someone-else"}
	e = inStoreEngine(t, &fakeRecorder{}, authz.RoleStaff, &stores.Store{}, r)
	if w := postReceipt(e, `{"send":true,"email":"jo@example.com"}`); w.Code != http.StatusOK || r.sent != "jo@example.com" {
		t.Errorf("staff on any sale: %d sent=%q", w.Code, r.sent)
	}
}

func TestReceiptLink_SendNeedsEmail(t *testing.T) {
	r := &fakeReceipts{soldBy: "u-1"}
	e := inStoreEngine(t, &fakeRecorder{}, authz.RoleCashier, &stores.Store{}, r)
	if w := postReceipt(e, `{"send":true}`); w.Code != http.StatusBadRequest {
		t.Errorf("send without email: %d", w.Code)
	}
}
