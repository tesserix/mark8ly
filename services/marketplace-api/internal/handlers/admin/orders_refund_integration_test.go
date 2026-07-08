//go:build integration

// Package admin_test — refund-specific fixtures for the admin orders
// integration suite. Task 9 wired OrdersHandler.Refund to
// orderrefund.Coordinator, which moves real money through a payment.Gateway.
// The fakeGateway/fakeRefundResolver here mirror the pattern used in
// internal/orderrefund's own integration tests (coordinator_integration_test.go)
// so the admin HTTP surface can be exercised end-to-end without ever
// touching a real Stripe/Razorpay account. setupOrdersRouter (in
// orders_integration_test.go, same package) wires every OrdersHandler in
// this suite with a Coordinator built from newTestRefundCoordinator.
package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/payment"
)

// fakeGateway implements payment.Gateway with a canned successful
// RefundPayment — refund tests must never dial out to a real provider.
type fakeGateway struct{}

func (f *fakeGateway) CreateIntent(ctx context.Context, in payment.CreateIntentInput) (*payment.Intent, error) {
	return nil, errors.New("fakeGateway: CreateIntent not implemented")
}

func (f *fakeGateway) CapturePayment(ctx context.Context, captureID string) (*payment.Capture, error) {
	return nil, errors.New("fakeGateway: CapturePayment not implemented")
}

func (f *fakeGateway) RefundPayment(ctx context.Context, in payment.RefundInput) (*payment.Refund, error) {
	return &payment.Refund{ProviderRefundID: "re_test", Status: "succeeded", Amount: in.Amount}, nil
}

func (f *fakeGateway) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*payment.WebhookEvent, error) {
	return nil, errors.New("fakeGateway: VerifyWebhook not implemented")
}

func (f *fakeGateway) ProviderName() string { return "stripe" }

func (f *fakeGateway) SupportedCountries() []string { return []string{"US"} }

// fakeRefundResolver embeds the real *orderrefund.Resolver so
// PaymentContextForOrder reads seeded rows for real, but overrides
// GatewayFor to hand back the fakeGateway instead of constructing a real
// provider client from decrypted API keys.
type fakeRefundResolver struct {
	*orderrefund.Resolver
	gw payment.Gateway
}

func (f *fakeRefundResolver) GatewayFor(ctx context.Context, storeID uuid.UUID, provider string) (payment.Gateway, error) {
	return f.gw, nil
}

// newTestRefundCoordinator wires a Coordinator against real payment/order
// services and a fakeRefundResolver, always enabled so the admin refund
// endpoint actually runs the saga in tests.
func newTestRefundCoordinator(db *gorm.DB, gw payment.Gateway) *orderrefund.Coordinator {
	res := &fakeRefundResolver{Resolver: orderrefund.NewResolver(db), gw: gw}
	pay := payment.NewService(payment.NewRepository(db))
	orders := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	return orderrefund.NewCoordinator(db, res, pay, orders, order.NewRepository(), true)
}

// seedCapturedPaymentTxn inserts a captured payment_transactions row so
// orderrefund.Resolver.PaymentContextForOrder resolves a refund target for
// orderID. Required before Coordinator.Refund will do anything but 422.
func seedCapturedPaymentTxn(t *testing.T, db *gorm.DB, tenantID, storeID, orderID uuid.UUID, amount string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO payment_transactions
			(id, tenant_id, store_id, order_id, provider, provider_intent_id, provider_payment_id, amount, currency_code, status, metadata, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, 'stripe', 'pi_test', 'pi_test', ?, 'USD', 'captured', '{}'::jsonb, now(), now())`,
		tenantID, storeID, orderID, amount,
	).Error
	if err != nil {
		t.Fatalf("seedCapturedPaymentTxn: %v", err)
	}
}

// seedActiveGatewayConfig inserts an active stripe payment_gateway_configs
// row so orderrefund.Resolver.GatewayFor resolves — the fake gateway
// injected via fakeRefundResolver.GatewayFor never reads these column
// values, but the row must exist and be active for resolution to succeed
// in the real (non-fake) codepath this mirrors.
func seedActiveGatewayConfig(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO payment_gateway_configs (id, tenant_id, store_id, provider, api_key_encrypted, secret_key_encrypted, mode, is_active)
		 VALUES (gen_random_uuid(), ?, ?, 'stripe', 'sk', 'sk_secret', 'test', true)`,
		tenantID, storeID,
	).Error
	if err != nil {
		t.Fatalf("seedActiveGatewayConfig: %v", err)
	}
}

// createAndConfirmOrder drives create → confirm(paid) over HTTP so the
// order reaches payment_status=paid the same way a real merchant would,
// and returns its id. grandTotal is used as both subtotal and grand_total
// (no shipping/tax) to keep the refund-cap math simple in callers.
func createAndConfirmOrder(t *testing.T, env *ordersTestEnv, base string, headers map[string]string, grandTotal string) string {
	t.Helper()
	createBody := map[string]any{
		"idempotency_key": "refund-fixture-" + uuid.NewString(),
		"customer_email":  "refund-fixture@example.com",
		"items": []map[string]any{{
			"title_snapshot": "Widget", "sku_snapshot": "WID-" + uuid.NewString()[:8],
			"unit_price": grandTotal, "quantity": 1, "line_total": grandTotal,
			"currency_code": "USD",
		}},
		"shipping":      map[string]any{"name": "A", "line1": "1 Main St", "city": "Dublin", "country_code": "IE"},
		"billing":       map[string]any{"name": "A", "line1": "1 Main St", "city": "Dublin", "country_code": "IE"},
		"subtotal":      grandTotal,
		"grand_total":   grandTotal,
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

	confirmBody := map[string]any{"payment_status": "paid", "reason": "stripe-auth"}
	w = request(t, env.router, http.MethodPost, base+"/"+created.ID+"/confirm", confirmBody, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: status %d body=%s", w.Code, w.Body.String())
	}
	return created.ID
}

// TestAPI_OrdersRefund_NoCapturedPayment_Unavailable asserts that refunding
// an order with no captured payment_transactions row is rejected before
// the gateway or the order's payment_status is ever touched — the
// coordinator returns apperrors.RefundUnavailable, which RespondErr maps
// to 422 with error code "refund_unavailable".
func TestAPI_OrdersRefund_NoCapturedPayment_Unavailable(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	// Deliberately do NOT seed payment_transactions or payment_gateway_configs.

	refundBody := map[string]any{"amount": "50.00", "reason": "customer-credit"}
	w := request(t, env.router, http.MethodPost, base+"/"+orderID+"/refund", refundBody, headers)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refund: status %d, want 422 (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("refund: unmarshal: %v", err)
	}
	if body["error"] != "refund_unavailable" {
		t.Fatalf("refund: error = %v, want refund_unavailable", body["error"])
	}
}

// TestAPI_OrdersRefund_OverCap_Rejected asserts that a refund request
// exceeding the captured total (min(grand_total, capturedTotal)) is
// rejected with apperrors.ErrRefundExceedsTotal (422,
// "refund_exceeds_total") and never reaches the gateway.
func TestAPI_OrdersRefund_OverCap_Rejected(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "100.00")
	seedCapturedPaymentTxn(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID), "100.00")
	seedActiveGatewayConfig(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID))

	refundBody := map[string]any{"amount": "999.00", "reason": "customer-credit"}
	w := request(t, env.router, http.MethodPost, base+"/"+orderID+"/refund", refundBody, headers)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refund: status %d, want 422 (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("refund: unmarshal: %v", err)
	}
	if body["error"] != "refund_exceeds_total" {
		t.Fatalf("refund: error = %v, want refund_exceeds_total", body["error"])
	}
}

// TestAPI_OrdersCancel_Paid_AutoRefunds asserts Task 11's admin-side hook:
// cancelling a paid order with a captured payment auto-refunds the full
// remaining balance through the same orderrefund.Coordinator saga the
// Refund endpoint uses. Best-effort — the cancel response is 200 either
// way — so this asserts on the resulting DB state (order + ledger row)
// rather than the response body.
func TestAPI_OrdersCancel_Paid_AutoRefunds(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	orderID := createAndConfirmOrder(t, env, base, headers, "75.00")
	seedCapturedPaymentTxn(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID), uuid.MustParse(orderID), "75.00")
	seedActiveGatewayConfig(t, env.db, uuid.MustParse(tenantID), uuid.MustParse(storeID))

	cancelBody := map[string]any{"reason": "merchant cancelled — out of stock"}
	w := request(t, env.router, http.MethodPost, base+"/"+orderID+"/cancel", cancelBody, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: status %d, body=%s", w.Code, w.Body.String())
	}

	var reloaded order.Order
	if err := env.db.Table("orders").First(&reloaded, "id = ?", orderID).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if reloaded.Status != string(order.OrderStatusCancelled) {
		t.Fatalf("order status = %q, want cancelled", reloaded.Status)
	}
	if reloaded.PaymentStatus != string(order.PaymentStatusRefunded) {
		t.Fatalf("order payment_status = %q, want refunded", reloaded.PaymentStatus)
	}

	var refundCount int64
	if err := env.db.Table("refund_transactions").
		Where("order_id = ? AND status = 'succeeded' AND idempotency_key = ?", orderID, "refund_"+orderID+"_cancel").
		Count(&refundCount).Error; err != nil {
		t.Fatalf("count refund_transactions: %v", err)
	}
	if refundCount != 1 {
		t.Fatalf("refund_transactions succeeded rows = %d, want 1", refundCount)
	}
}
