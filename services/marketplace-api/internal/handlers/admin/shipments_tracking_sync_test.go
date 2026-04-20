//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// trackingSyncTables is the focused truncate list for the sync test.
// CASCADE cleans up leftover rows between runs; order here is the
// dependency graph, children first.
var trackingSyncTables = []string{
	"order_events",
	"shipments",
	"shipping_carrier_configs",
	"stores",
}

// scriptedCarrier returns a deterministic sequence of statuses per
// tracking-number on successive calls. It's the entire point of the
// test: drive the sync loop through the "in_transit" →
// "out_for_delivery" → "delivered" ladder and assert every rung
// produces the correct side-effects.
type scriptedCarrier struct {
	sequence []string
	calls    atomic.Int32
}

func (c *scriptedCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) {
	return nil, fmt.Errorf("not implemented")
}
func (c *scriptedCarrier) CreateShipment(context.Context, shipping.ShipmentRequest) (*shipping.Shipment, error) {
	return nil, fmt.Errorf("not implemented")
}
func (c *scriptedCarrier) CancelShipment(context.Context, string) error {
	return fmt.Errorf("not implemented")
}
func (c *scriptedCarrier) ProviderName() string        { return "delhivery" }
func (c *scriptedCarrier) SupportedCountries() []string { return []string{"IN"} }

func (c *scriptedCarrier) GetTracking(_ context.Context, trackingNumber string) (*shipping.Tracking, error) {
	idx := int(c.calls.Add(1)) - 1
	if idx >= len(c.sequence) {
		idx = len(c.sequence) - 1
	}
	status := c.sequence[idx]
	return &shipping.Tracking{
		TrackingNumber: trackingNumber,
		Status:         status,
		Events: []shipping.TrackingEvent{{
			Description: "scripted-" + status,
			Timestamp:   time.Now().UTC(),
		}},
	}, nil
}

// TestShipmentsSync_AdvancesStatusLadder pins the end-to-end behaviour
// of the 2-minute background poller: each successive carrier call
// must cascade into a new status on the row, stamp the appropriate
// timestamp, and append exactly one order_events entry per transition.
// Failures here mean either the poller stalled, double-fired, or
// regressed the status (which would unwind the delivered-email
// side-effect too).
func TestShipmentsSync_AdvancesStatusLadder(t *testing.T) {
	db := testdb.NewDB(t, trackingSyncTables...)

	// Seed: one store, one carrier config (tracks the api_key / mode
	// the sync loop needs), one open shipment bound to an order.
	tenantID := uuid.New()
	storeID := uuid.New()
	orderID := uuid.New()
	awb := "TRACK-SYNC-1"

	seedStoreRowForSync(t, db, tenantID, storeID)

	// Encrypt a dummy api_key so the handler exercises the decryption
	// branch — matches the regression pinned by shipments_decrypt_test.
	enc := crypto.NewNoopEncryptor()
	apiCipher, err := enc.Encrypt("plaintext-token")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	cfg := shipping.CarrierConfig{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		Provider: "delhivery", APIKey: apiCipher, SecretKey: "",
		Mode: "test", Enabled: true,
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}
	shipmentID := uuid.New()
	ship := shipping.ShipmentRecord{
		ID: shipmentID, TenantID: tenantID, StoreID: storeID, OrderID: orderID,
		Carrier: "delhivery", TrackingNumber: awb, Status: "pending",
		ShipFrom:     datatypes.JSON([]byte(`{}`)),
		ShipTo:       datatypes.JSON([]byte(`{}`)),
		HandlingFee:  decimal.Zero,
		CurrencyCode: "INR",
	}
	if err := db.Create(&ship).Error; err != nil {
		t.Fatalf("seed shipment: %v", err)
	}
	// Also minimally seed orders so any FK constraint from order_events
	// is satisfied. The sync doesn't read orders directly but
	// order_events.order_id references orders(id).
	seedOrderRowForSync(t, db, orderID, storeID, tenantID)

	// Build the handler with a fake carrier that serves the ladder.
	shippingRepo := shipping.NewRepository(db)
	shippingService := shipping.NewShippingService(shippingRepo)
	carrier := &scriptedCarrier{sequence: []string{"in_transit", "out_for_delivery", "delivered"}}
	h := admin.NewShipmentsHandler(db, shippingService, shippingRepo, nil, nil).
		WithEncryptor(enc).
		WithCarrierFactory(func(provider string, _ any) (shipping.Carrier, bool) {
			if strings.ToLower(provider) == "delhivery" {
				return carrier, true
			}
			return nil, false
		})

	// Invocation 1 — expect in_transit.
	if got := runSyncOnce(t, h); got.Updated != 1 {
		t.Fatalf("pass 1: updated = %d, want 1 (summary=%+v)", got.Updated, got)
	}
	requireShipmentStatus(t, db, shipmentID, "in_transit")
	requireShippedAtSet(t, db, shipmentID)
	requireEventCount(t, db, orderID, "shipment_in_transit", 1)

	// Invocation 2 — expect out_for_delivery.
	if got := runSyncOnce(t, h); got.Updated != 1 {
		t.Fatalf("pass 2: updated = %d, want 1 (summary=%+v)", got.Updated, got)
	}
	requireShipmentStatus(t, db, shipmentID, "out_for_delivery")
	requireEventCount(t, db, orderID, "shipment_out_for_delivery", 1)

	// Invocation 3 — expect delivered, delivered_at stamped.
	if got := runSyncOnce(t, h); got.Updated != 1 {
		t.Fatalf("pass 3: updated = %d, want 1 (summary=%+v)", got.Updated, got)
	}
	requireShipmentStatus(t, db, shipmentID, "delivered")
	requireDeliveredAtSet(t, db, shipmentID)
	requireEventCount(t, db, orderID, "shipment_delivered", 1)

	// Invocation 4 — terminal, must NOT be picked up again. examined
	// should drop to 0 because the WHERE clause excludes "delivered".
	got := runSyncOnce(t, h)
	if got.Examined != 0 {
		t.Fatalf("terminal pass: examined = %d, want 0", got.Examined)
	}
}

// TestShipmentsSync_CarrierErrorDoesNotBlockOthers verifies that one
// failing carrier call doesn't poison the rest of the run — a single
// Delhivery timeout on shipment A must not stop the handler from
// advancing shipment B. Without this guarantee, any flaky shipment
// would freeze the entire 2-minute sync.
func TestShipmentsSync_CarrierErrorDoesNotBlockOthers(t *testing.T) {
	db := testdb.NewDB(t, trackingSyncTables...)

	tenantID := uuid.New()
	storeID := uuid.New()
	seedStoreRowForSync(t, db, tenantID, storeID)

	enc := crypto.NewNoopEncryptor()
	apiCipher, _ := enc.Encrypt("plaintext")
	cfg := shipping.CarrierConfig{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		Provider: "delhivery", APIKey: apiCipher, Mode: "test", Enabled: true,
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// Two open shipments — A fails, B succeeds.
	orderA := uuid.New()
	orderB := uuid.New()
	shipA := uuid.New()
	shipB := uuid.New()
	for _, s := range []shipping.ShipmentRecord{
		{ID: shipA, TenantID: tenantID, StoreID: storeID, OrderID: orderA,
			Carrier: "delhivery", TrackingNumber: "FAIL-1", Status: "pending",
			ShipFrom: datatypes.JSON([]byte(`{}`)), ShipTo: datatypes.JSON([]byte(`{}`)),
			CurrencyCode: "INR"},
		{ID: shipB, TenantID: tenantID, StoreID: storeID, OrderID: orderB,
			Carrier: "delhivery", TrackingNumber: "OK-1", Status: "pending",
			ShipFrom: datatypes.JSON([]byte(`{}`)), ShipTo: datatypes.JSON([]byte(`{}`)),
			CurrencyCode: "INR"},
	} {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seedOrderRowForSync(t, db, orderA, storeID, tenantID)
	seedOrderRowForSync(t, db, orderB, storeID, tenantID)

	shippingRepo := shipping.NewRepository(db)
	h := admin.NewShipmentsHandler(db, shipping.NewShippingService(shippingRepo), shippingRepo, nil, nil).
		WithEncryptor(enc).
		WithCarrierFactory(func(_ string, _ any) (shipping.Carrier, bool) {
			return &mixedCarrier{}, true
		})

	got := runSyncOnce(t, h)
	if got.Errors != 1 {
		t.Fatalf("errors = %d, want 1 (summary=%+v)", got.Errors, got)
	}
	if got.Updated != 1 {
		t.Fatalf("updated = %d, want 1 (summary=%+v)", got.Updated, got)
	}
	requireShipmentStatus(t, db, shipB, "in_transit")
}

type mixedCarrier struct{}

func (mixedCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) {
	return nil, nil
}
func (mixedCarrier) CreateShipment(context.Context, shipping.ShipmentRequest) (*shipping.Shipment, error) {
	return nil, nil
}
func (mixedCarrier) CancelShipment(context.Context, string) error { return nil }
func (mixedCarrier) ProviderName() string                         { return "delhivery" }
func (mixedCarrier) SupportedCountries() []string                 { return []string{"IN"} }
func (mixedCarrier) GetTracking(_ context.Context, awb string) (*shipping.Tracking, error) {
	if strings.HasPrefix(awb, "FAIL-") {
		return nil, fmt.Errorf("synthetic carrier failure")
	}
	return &shipping.Tracking{TrackingNumber: awb, Status: "in_transit"}, nil
}

// runSyncOnce invokes the sync handler via an httptest round-trip so
// the test exercises the same code path the CronJob does (including
// JSON serialization). Returns the decoded summary.
func runSyncOnce(t *testing.T, h *admin.ShipmentsHandler) admin.SyncSummary {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal/shipments/tracking/sync", h.SyncAllOpenShipments)
	req := httptest.NewRequest(http.MethodPost, "/internal/shipments/tracking/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync: status = %d, body = %s", w.Code, w.Body.String())
	}
	var sum admin.SyncSummary
	if err := json.Unmarshal(w.Body.Bytes(), &sum); err != nil {
		t.Fatalf("sync decode: %v", err)
	}
	return sum
}

// ----- seed + assertion helpers ------------------------------------

func seedStoreRowForSync(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	// Direct SQL so we don't depend on the stores package test helpers —
	// this file can run even when the stores projection table has
	// evolved. Field list chosen to satisfy NOT NULL columns in
	// migrations/000002_orders_initial.up.sql.
	err := db.Exec(`
		INSERT INTO stores (id, tenant_id, name, slug, country_code, currency_code, created_at, updated_at)
		VALUES (?, ?, 'Test', ?, 'IN', 'INR', now(), now())
		ON CONFLICT (id) DO NOTHING`,
		storeID, tenantID, "test-"+storeID.String()[:8]).Error
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
}

func seedOrderRowForSync(t *testing.T, db *gorm.DB, orderID, storeID, tenantID uuid.UUID) {
	t.Helper()
	// Minimal order row to satisfy FK from order_events.order_id. The
	// sync loop doesn't read this row; we're just keeping referential
	// integrity happy.
	err := db.Exec(`
		INSERT INTO orders (id, tenant_id, store_id, order_number, customer_email,
			status, payment_status, fulfillment_status,
			subtotal, tax_total, shipping_total, grand_total, refunded_amount,
			currency_code, placed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'test@example.com',
			'pending', 'pending', 'pending',
			'0', '0', '0', '0', '0',
			'INR', now(), now(), now())
		ON CONFLICT (id) DO NOTHING`,
		orderID, tenantID, storeID, "M-SYNC-"+orderID.String()[:8]).Error
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
}

func requireShipmentStatus(t *testing.T, db *gorm.DB, shipmentID uuid.UUID, want string) {
	t.Helper()
	var got string
	if err := db.Raw(`SELECT status FROM shipments WHERE id = ?`, shipmentID).Scan(&got).Error; err != nil {
		t.Fatalf("read shipment: %v", err)
	}
	if got != want {
		t.Fatalf("shipment status = %q, want %q", got, want)
	}
}

func requireShippedAtSet(t *testing.T, db *gorm.DB, shipmentID uuid.UUID) {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM shipments WHERE id = ? AND shipped_at IS NOT NULL`, shipmentID).Scan(&n).Error; err != nil {
		t.Fatalf("shipped_at check: %v", err)
	}
	if n != 1 {
		t.Fatalf("shipped_at not stamped on shipment %s", shipmentID)
	}
}

func requireDeliveredAtSet(t *testing.T, db *gorm.DB, shipmentID uuid.UUID) {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM shipments WHERE id = ? AND delivered_at IS NOT NULL`, shipmentID).Scan(&n).Error; err != nil {
		t.Fatalf("delivered_at check: %v", err)
	}
	if n != 1 {
		t.Fatalf("delivered_at not stamped on shipment %s", shipmentID)
	}
}

func requireEventCount(t *testing.T, db *gorm.DB, orderID uuid.UUID, kind string, want int) {
	t.Helper()
	var n int64
	if err := db.Model(&order.OrderEvent{}).
		Where("order_id = ? AND kind = ?", orderID, kind).
		Count(&n).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if int(n) != want {
		t.Fatalf("order_events count for kind=%s on order %s = %d, want %d",
			kind, orderID, n, want)
	}
}
