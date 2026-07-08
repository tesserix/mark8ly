//go:build integration

package admin_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ordersTables is the dependency-ordered truncate list for orders tests.
// CASCADE handles the rest.
var ordersTables = []string{
	"order_events",
	"order_addresses",
	"order_items",
	"return_items",
	"returns",
	"refund_transactions",
	"payment_transactions",
	"payment_gateway_configs",
	"orders",
	"abandoned_carts",
	"outbox_events",
	"store_watermarks",
	"stores",
}

// ordersTestEnv is a focused router scaffold for the orders endpoint suite.
// It wires only what the orders handlers need (no products/categories/media).
type ordersTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	fga    *authz.FakeClient
}

func setupOrdersRouter(t *testing.T) *ordersTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, ordersTables...)

	storesRepo := stores.NewRepository(db)
	outboxRepo := outbox.NewRepository(db)
	orderRepo := order.NewRepository()
	returnRepo := order.NewReturnRepository()
	abandonedCartRepo := order.NewAbandonedCartRepository()

	orderSvc := order.NewService(db, orderRepo, outboxRepo)
	returnSvc := order.NewReturnService(db, returnRepo, orderRepo, orderSvc, outboxRepo)
	abandonedCartSvc := order.NewAbandonedCartService(db, abandonedCartRepo, outboxRepo)

	fga := authz.NewFakeClient()
	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   storesRepo,
		Client: stubClient{},
		Flight: &singleflight.Group{},
	})
	authzMW := authz.NewMiddleware(fga, nil)

	// Refund coordinator wired against a fake gateway so refund tests never
	// hit real Stripe. fakeGateway/fakeRefundResolver live in
	// orders_refund_integration_test.go (same package/build tag).
	gw := &fakeGateway{}
	refundCoordinator := newTestRefundCoordinator(db, gw)

	ordersHandler := admin.NewOrdersHandler(db, orderSvc, orderRepo, nil, nil).
		WithRefunds(refundCoordinator)
	returnsHandler := admin.NewReturnsHandler(db, returnSvc, returnRepo, orderRepo, orderSvc, nil).
		WithRefunds(refundCoordinator)
	abandonedCartsHandler := admin.NewAbandonedCartsHandler(abandonedCartSvc, nil)

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		OrdersHandler:         ordersHandler,
		ReturnsHandler:        returnsHandler,
		AbandonedCartsHandler: abandonedCartsHandler,
		StoresMiddleware:      storeMW,
		AuthzMiddleware:       authzMW,
		InternalSecret:        "",
	})

	return &ordersTestEnv{router: r, db: db, fga: fga}
}

// TestAPI_OrdersHappyPath drives the full orders endpoint surface end-to-end:
// create → confirm → fulfill → refund. Asserts HTTP status codes and that
// the order's final state in the response body reflects every transition.
func TestAPI_OrdersHappyPath(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	// --- Create ---
	createBody := map[string]any{
		"idempotency_key": "happy-" + uuid.NewString(),
		"customer_email":  "happy@example.com",
		"items": []map[string]any{
			{
				"title_snapshot": "Widget",
				"sku_snapshot":   "WID-1",
				"unit_price":     "50.00",
				"quantity":       2,
				"line_total":     "100.00",
				"currency_code":  "USD",
			},
		},
		"shipping": map[string]any{
			"name": "A", "line1": "1 Main St", "city": "Dublin", "country_code": "IE",
		},
		"billing": map[string]any{
			"name": "A", "line1": "1 Main St", "city": "Dublin", "country_code": "IE",
		},
		"subtotal":      "100.00",
		"grand_total":   "100.00",
		"currency_code": "USD",
	}
	w := request(t, env.router, http.MethodPost, base, createBody, headers)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d body=%s", w.Code, w.Body.String())
	}
	var created admin.AdminOrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("create: unmarshal: %v", err)
	}
	if created.Status != "pending" {
		t.Fatalf("create: status = %q, want pending", created.Status)
	}
	if !strings.HasPrefix(created.OrderNumber, "M-") {
		t.Fatalf("create: order_number = %q, want M-… prefix", created.OrderNumber)
	}
	orderID := created.ID

	// --- Idempotent replay returns 200 with the same id ---
	w = request(t, env.router, http.MethodPost, base, createBody, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("replay: status %d body=%s", w.Code, w.Body.String())
	}
	var replay admin.AdminOrderResponse
	_ = json.Unmarshal(w.Body.Bytes(), &replay)
	if replay.ID != orderID {
		t.Fatalf("replay: id %q != original %q", replay.ID, orderID)
	}

	// --- Confirm with payment status ---
	confirmBody := map[string]any{
		"payment_status": "paid",
		"reason":         "stripe-auth",
	}
	w = request(t, env.router, http.MethodPost, base+"/"+orderID+"/confirm", confirmBody, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: status %d body=%s", w.Code, w.Body.String())
	}
	var confirmed admin.AdminOrderResponse
	_ = json.Unmarshal(w.Body.Bytes(), &confirmed)
	if confirmed.Status != "confirmed" {
		t.Fatalf("confirm: status = %q, want confirmed", confirmed.Status)
	}
	if confirmed.PaymentStatus != "paid" {
		t.Fatalf("confirm: payment_status = %q, want paid", confirmed.PaymentStatus)
	}

	// --- Fulfill ---
	w = request(t, env.router, http.MethodPost, base+"/"+orderID+"/fulfill", nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("fulfill: status %d body=%s", w.Code, w.Body.String())
	}
	var fulfilled admin.AdminOrderResponse
	_ = json.Unmarshal(w.Body.Bytes(), &fulfilled)
	if fulfilled.Status != "fulfilled" {
		t.Fatalf("fulfill: status = %q, want fulfilled", fulfilled.Status)
	}
	if fulfilled.FulfillmentStatus != "fulfilled" {
		t.Fatalf("fulfill: fulfillment_status = %q, want fulfilled", fulfilled.FulfillmentStatus)
	}

	// --- Partial refund (50 of 100) ---
	// Refunds now move real money through the coordinator, which requires
	// a captured gateway transaction + an active gateway config to resolve
	// the payment context and provider — seed both before hitting /refund.
	seedCapturedPaymentTxn(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID), "100.00")
	seedActiveGatewayConfig(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID))
	refundBody := map[string]any{
		"amount":            "50.00",
		"reason":            "customer-credit",
		"refund_request_id": uuid.NewString(),
	}
	w = request(t, env.router, http.MethodPost, base+"/"+orderID+"/refund", refundBody, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("refund: status %d body=%s", w.Code, w.Body.String())
	}
	var refunded admin.AdminOrderResponse
	_ = json.Unmarshal(w.Body.Bytes(), &refunded)
	if refunded.PaymentStatus != "partially_refunded" {
		t.Fatalf("refund: payment_status = %q, want partially_refunded", refunded.PaymentStatus)
	}
	if refunded.RefundedAmount.String() != "50" && refunded.RefundedAmount.String() != "50.00" {
		t.Fatalf("refund: refunded_amount = %q, want 50", refunded.RefundedAmount.String())
	}
	// RefundOrderResponse anonymously embeds AdminOrderResponse and adds
	// provider_refund_id — unmarshal separately into a narrow struct to
	// assert the gateway's fakeGateway-supplied id round-trips.
	var refundMeta struct {
		ProviderRefundID string `json:"provider_refund_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &refundMeta)
	if refundMeta.ProviderRefundID != "re_test" {
		t.Fatalf("refund: provider_refund_id = %q, want re_test", refundMeta.ProviderRefundID)
	}

	// --- Get returns the same final state ---
	w = request(t, env.router, http.MethodGet, base+"/"+orderID, nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("get: status %d body=%s", w.Code, w.Body.String())
	}

	// --- List shows the order in pagination meta ---
	w = request(t, env.router, http.MethodGet, base+"?page=1&page_size=10", nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("list: status %d body=%s", w.Code, w.Body.String())
	}
	var listResp struct {
		Data []admin.AdminOrderResponse `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Meta.Total < 1 {
		t.Fatalf("list: total = %d, want ≥1", listResp.Meta.Total)
	}
}

// TestAPI_OrdersAuthDenied verifies that a viewer-only role cannot mutate
// orders — the cross-tenant denial mechanism is exercised generically by
// the existing middleware tests, so this test just covers the role-gating
// edge case for the orders surface specifically.
func TestAPI_OrdersAuthDenied(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	// Grant only Viewer — below the OrdersEditRole threshold (Admin).
	env.fga.Grant(userID, authz.RoleViewer, tenantID)

	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	body := map[string]any{
		"idempotency_key": "denied-" + uuid.NewString(),
		"customer_email":  "denied@example.com",
		"items": []map[string]any{{
			"title_snapshot": "x", "sku_snapshot": "x",
			"unit_price": "1.00", "quantity": 1, "line_total": "1.00",
			"currency_code": "USD",
		}},
		"shipping":      map[string]any{"name": "A", "line1": "1", "city": "C", "country_code": "IE"},
		"billing":       map[string]any{"name": "A", "line1": "1", "city": "C", "country_code": "IE"},
		"subtotal":      "1.00",
		"grand_total":   "1.00",
		"currency_code": "USD",
	}
	w := request(t, env.router, http.MethodPost, base, body, headers)
	// 404 not 403 per spec §13.1.1 — no existence leaks across tenants.
	if w.Code != http.StatusNotFound {
		t.Fatalf("denied create: status %d, want 404 (body=%s)", w.Code, w.Body.String())
	}
}

// silence unused import linter — time is referenced indirectly via DTOs.
var _ = time.Now
