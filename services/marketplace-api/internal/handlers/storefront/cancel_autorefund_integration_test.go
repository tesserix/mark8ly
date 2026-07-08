//go:build integration

// Package storefront_test — Task 11: auto-refund a PAID order when the
// customer self-cancels it (spec §4). Mirrors the fakeGateway/
// fakeRefundResolver pattern used by the admin refund integration suite
// (internal/handlers/admin/orders_refund_integration_test.go) so this test
// exercises the real orderrefund.Coordinator saga — reserve pending ledger
// row, "call" the gateway, finalize — without ever dialing out to Stripe.
package storefront_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// cancelAutoRefundTables is the dependency-ordered truncate list for this
// suite. CASCADE handles cross-table cleanup.
var cancelAutoRefundTables = []string{
	"order_events",
	"order_addresses",
	"order_items",
	"shipments",
	"refund_transactions",
	"payment_transactions",
	"payment_gateway_configs",
	"orders",
	"customer_profiles",
	"outbox_events",
	"store_watermarks",
	"stores",
}

// carFakeGateway implements payment.Gateway with a canned successful
// RefundPayment — this suite must never dial out to a real provider.
// Named distinctly from admin_test's fakeGateway since these live in
// different packages but the same binary during `go vet ./...`.
type carFakeGateway struct{}

func (f *carFakeGateway) CreateIntent(ctx context.Context, in payment.CreateIntentInput) (*payment.Intent, error) {
	return nil, errors.New("carFakeGateway: CreateIntent not implemented")
}

func (f *carFakeGateway) CapturePayment(ctx context.Context, captureID string) (*payment.Capture, error) {
	return nil, errors.New("carFakeGateway: CapturePayment not implemented")
}

func (f *carFakeGateway) RefundPayment(ctx context.Context, in payment.RefundInput) (*payment.Refund, error) {
	return &payment.Refund{ProviderRefundID: "re_test_cancel", Status: "succeeded", Amount: in.Amount}, nil
}

func (f *carFakeGateway) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*payment.WebhookEvent, error) {
	return nil, errors.New("carFakeGateway: VerifyWebhook not implemented")
}

func (f *carFakeGateway) ProviderName() string { return "stripe" }

func (f *carFakeGateway) SupportedCountries() []string { return []string{"US"} }

// carFakeRefundResolver embeds the real *orderrefund.Resolver so
// PaymentContextForOrder reads seeded rows for real, but overrides
// GatewayFor to hand back the fake gateway instead of constructing a real
// provider client from decrypted API keys.
type carFakeRefundResolver struct {
	*orderrefund.Resolver
	gw payment.Gateway
}

func (f *carFakeRefundResolver) GatewayFor(ctx context.Context, storeID uuid.UUID, provider string) (payment.Gateway, error) {
	return f.gw, nil
}

// cancelAutoRefundEnv bundles the router, db, and seeded store for this suite.
type cancelAutoRefundEnv struct {
	db      *gorm.DB
	handler *storefront.OrderDetailHandler
	store   *stores.Store
}

// setupCancelAutoRefundEnv wires an OrderDetailHandler with orderSvc + a
// refund Coordinator backed by the fake gateway above, always enabled so
// the auto-refund path actually runs the saga in tests.
func setupCancelAutoRefundEnv(t *testing.T) *cancelAutoRefundEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, cancelAutoRefundTables...)

	store := seedCancelAutoRefundStore(t, db)

	orderRepo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	orderSvc := order.NewService(db, orderRepo, outboxRepo)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	gw := &carFakeGateway{}
	res := &carFakeRefundResolver{Resolver: orderrefund.NewResolver(db), gw: gw}
	paySvc := payment.NewService(payment.NewRepository(db))
	refundCoordinator := orderrefund.NewCoordinator(db, res, paySvc, orderSvc, orderRepo, true)

	handler := storefront.NewOrderDetailHandler(db, orderRepo, orderSvc, nil, logger).
		WithRefunds(refundCoordinator)

	return &cancelAutoRefundEnv{db: db, handler: handler, store: store}
}

// buildCancelRouter mounts a minimal router exercising only Cancel, with a
// tiny middleware that sets "store" and storefront.CustomerProfileKey on
// the gin context — mimicking what StoreContext + OptionalCustomerAuth do
// in production, without the overhead of real signed session cookies.
func buildCancelRouter(handler *storefront.OrderDetailHandler, store *stores.Store, profile *customer.CustomerProfile) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("store", store)
		if profile != nil {
			c.Set(storefront.CustomerProfileKey, profile)
		}
		c.Next()
	})
	r.POST("/storefront/stores/:storeSlug/orders/:id/cancel", handler.Cancel)
	return r
}

func seedCancelAutoRefundStore(t *testing.T, db *gorm.DB) *stores.Store {
	t.Helper()
	storeID := uuid.NewString()
	tenantID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "car-" + storeID[:8],
		Name:         "Cancel Auto-Refund Test Store",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return s
}

func seedCancelAutoRefundCustomer(t *testing.T, db *gorm.DB, store *stores.Store) *customer.CustomerProfile {
	t.Helper()
	p := &customer.CustomerProfile{
		TenantID: uuid.MustParse(store.TenantID),
		StoreID:  uuid.MustParse(store.ID),
		Email:    "cancel-autorefund-" + uuid.NewString()[:8] + "@example.com",
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("seed customer profile: %v", err)
	}
	return p
}

// cancelAutoRefundOrderOpts controls the seeded order's payment state.
type cancelAutoRefundOrderOpts struct {
	PaymentStatus string // defaults to order.PaymentStatusPaid
	GrandTotal    string // defaults to "120.00"
}

func seedCancelAutoRefundOrder(t *testing.T, db *gorm.DB, store *stores.Store, cust *customer.CustomerProfile, opts cancelAutoRefundOrderOpts) *order.Order {
	t.Helper()
	if opts.PaymentStatus == "" {
		opts.PaymentStatus = string(order.PaymentStatusPaid)
	}
	if opts.GrandTotal == "" {
		opts.GrandTotal = "120.00"
	}
	grandTotal, err := decimal.NewFromString(opts.GrandTotal)
	if err != nil {
		t.Fatalf("parse grand total: %v", err)
	}
	custID := cust.ID
	o := &order.Order{
		TenantID:       uuid.MustParse(store.TenantID),
		StoreID:        uuid.MustParse(store.ID),
		OrderNumber:    "M-" + uuid.NewString()[:8],
		IdempotencyKey: "cancel-autorefund-" + uuid.NewString(),
		CustomerID:     &custID,
		CustomerEmail:  cust.Email,
		Status:         string(order.OrderStatusConfirmed),
		PaymentStatus:  opts.PaymentStatus,
		Subtotal:       grandTotal,
		GrandTotal:     grandTotal,
		CurrencyCode:   "USD",
	}
	if err := db.Create(o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return o
}

// seedCancelAutoRefundCapturedTxn inserts a captured payment_transactions
// row so orderrefund.Resolver.PaymentContextForOrder resolves a refund
// target for orderID.
func seedCancelAutoRefundCapturedTxn(t *testing.T, db *gorm.DB, store *stores.Store, orderID uuid.UUID, amount string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO payment_transactions
			(id, tenant_id, store_id, order_id, provider, provider_intent_id, provider_payment_id, amount, currency_code, status, metadata, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, 'stripe', 'pi_x', 'pi_x', ?, 'USD', 'captured', '{}'::jsonb, now(), now())`,
		store.TenantID, store.ID, orderID, amount,
	).Error
	if err != nil {
		t.Fatalf("seed captured payment txn: %v", err)
	}
}

func seedCancelAutoRefundGatewayConfig(t *testing.T, db *gorm.DB, store *stores.Store) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO payment_gateway_configs (id, tenant_id, store_id, provider, api_key_encrypted, secret_key_encrypted, mode, is_active)
		 VALUES (gen_random_uuid(), ?, ?, 'stripe', 'sk', 'sk_secret', 'test', true)`,
		store.TenantID, store.ID,
	).Error
	if err != nil {
		t.Fatalf("seed gateway config: %v", err)
	}
}

func seedCancelAutoRefundShipment(t *testing.T, db *gorm.DB, store *stores.Store, orderID uuid.UUID) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO shipments (id, tenant_id, store_id, order_id, carrier, status, ship_from, ship_to, currency_code, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, 'usps', 'pending', '{}'::jsonb, '{}'::jsonb, 'USD', now(), now())`,
		store.TenantID, store.ID, orderID,
	).Error
	if err != nil {
		t.Fatalf("seed shipment: %v", err)
	}
}

func cancelAutoRefundRequest(r http.Handler, storeSlug, orderID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/storefront/stores/"+storeSlug+"/orders/"+orderID+"/cancel", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestCancelAutoRefund_Paid_TriggersFullRefund asserts that self-cancelling
// a paid, unshipped order (1) cancels the order and (2) auto-refunds the
// full grand_total through the (fake) gateway: a succeeded ledger row with
// the deterministic idempotency key, refunded_amount == grand_total, and
// payment_status flipped to refunded.
func TestCancelAutoRefund_Paid_TriggersFullRefund(t *testing.T) {
	env := setupCancelAutoRefundEnv(t)
	cust := seedCancelAutoRefundCustomer(t, env.db, env.store)
	o := seedCancelAutoRefundOrder(t, env.db, env.store, cust, cancelAutoRefundOrderOpts{GrandTotal: "120.00"})
	seedCancelAutoRefundCapturedTxn(t, env.db, env.store, o.ID, "120.00")
	seedCancelAutoRefundGatewayConfig(t, env.db, env.store)

	r := buildCancelRouter(env.handler, env.store, cust)
	w := cancelAutoRefundRequest(r, env.store.Slug, o.ID.String())
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: status %d, body=%s", w.Code, w.Body.String())
	}

	var reloaded order.Order
	if err := env.db.Table("orders").First(&reloaded, "id = ?", o.ID).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if reloaded.Status != string(order.OrderStatusCancelled) {
		t.Fatalf("order status = %q, want cancelled", reloaded.Status)
	}
	if reloaded.PaymentStatus != string(order.PaymentStatusRefunded) {
		t.Fatalf("order payment_status = %q, want refunded", reloaded.PaymentStatus)
	}
	if !reloaded.RefundedAmount.Equal(decimal.RequireFromString("120.00")) {
		t.Fatalf("order refunded_amount = %s, want 120.00", reloaded.RefundedAmount.String())
	}

	var refundCount int64
	if err := env.db.Table("refund_transactions").
		Where("order_id = ? AND status = 'succeeded' AND idempotency_key = ?", o.ID, "refund_"+o.ID.String()+"_cancel").
		Count(&refundCount).Error; err != nil {
		t.Fatalf("count refund_transactions: %v", err)
	}
	if refundCount != 1 {
		t.Fatalf("refund_transactions succeeded rows = %d, want 1", refundCount)
	}
}

// TestCancelAutoRefund_Unpaid_NoRefund asserts that self-cancelling a
// not-yet-paid order cancels it without ever attempting a refund — no
// refund_transactions row should appear.
func TestCancelAutoRefund_Unpaid_NoRefund(t *testing.T) {
	env := setupCancelAutoRefundEnv(t)
	cust := seedCancelAutoRefundCustomer(t, env.db, env.store)
	o := seedCancelAutoRefundOrder(t, env.db, env.store, cust, cancelAutoRefundOrderOpts{
		PaymentStatus: string(order.PaymentStatusPending),
		GrandTotal:    "80.00",
	})
	// Deliberately do NOT seed payment_transactions or payment_gateway_configs.

	r := buildCancelRouter(env.handler, env.store, cust)
	w := cancelAutoRefundRequest(r, env.store.Slug, o.ID.String())
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: status %d, body=%s", w.Code, w.Body.String())
	}

	var reloaded order.Order
	if err := env.db.Table("orders").First(&reloaded, "id = ?", o.ID).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if reloaded.Status != string(order.OrderStatusCancelled) {
		t.Fatalf("order status = %q, want cancelled", reloaded.Status)
	}

	var refundCount int64
	if err := env.db.Table("refund_transactions").Where("order_id = ?", o.ID).Count(&refundCount).Error; err != nil {
		t.Fatalf("count refund_transactions: %v", err)
	}
	if refundCount != 0 {
		t.Fatalf("refund_transactions rows = %d, want 0 for an unpaid order", refundCount)
	}
}

// TestCancelAutoRefund_ShipmentGuardWins asserts that the pre-existing
// shipment-in-flight guard still bounces self-cancel with 409 before any
// refund is attempted, even for a paid order.
func TestCancelAutoRefund_ShipmentGuardWins(t *testing.T) {
	env := setupCancelAutoRefundEnv(t)
	cust := seedCancelAutoRefundCustomer(t, env.db, env.store)
	o := seedCancelAutoRefundOrder(t, env.db, env.store, cust, cancelAutoRefundOrderOpts{GrandTotal: "50.00"})
	seedCancelAutoRefundCapturedTxn(t, env.db, env.store, o.ID, "50.00")
	seedCancelAutoRefundGatewayConfig(t, env.db, env.store)
	seedCancelAutoRefundShipment(t, env.db, env.store, o.ID)

	r := buildCancelRouter(env.handler, env.store, cust)
	w := cancelAutoRefundRequest(r, env.store.Slug, o.ID.String())
	if w.Code != http.StatusConflict {
		t.Fatalf("cancel: status %d, want 409, body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "shipment_in_flight" {
		t.Fatalf("error = %v, want shipment_in_flight", body["error"])
	}

	var reloaded order.Order
	if err := env.db.Table("orders").First(&reloaded, "id = ?", o.ID).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if reloaded.Status == string(order.OrderStatusCancelled) {
		t.Fatalf("order should NOT be cancelled when the shipment guard fires")
	}

	var refundCount int64
	if err := env.db.Table("refund_transactions").Where("order_id = ?", o.ID).Count(&refundCount).Error; err != nil {
		t.Fatalf("count refund_transactions: %v", err)
	}
	if refundCount != 0 {
		t.Fatalf("refund_transactions rows = %d, want 0 when the shipment guard blocks cancel", refundCount)
	}
}
