//go:build integration

// Package admin — coverage for creating one shipment per contributing
// warehouse (#177 PR 4b).
//
// An order with NO allocations must behave exactly as it did before this
// change: one shipment, pickup from the carrier config's warehouse. That is
// not a fallback for tidiness — order_allocations is empty in production, so
// it is the only path that currently runs.
//
// Create() constructs its carrier directly via shipping.NewCarrier, so it
// cannot be exercised without a live carrier account. WithCarrierConstructor
// (Step 0 of this task's brief) is the test-only seam that lets these tests
// substitute stubCarrier below instead.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ─── shared read helpers (from the brief) ──────────────────────────────

// groupsForOrder returns (warehouse_id, quantity) per allocation group,
// ordered for stable assertions.
func groupsForOrder(t *testing.T, db *gorm.DB, orderID string) []struct {
	WarehouseID string
	Quantity    int
	ShipmentID  *string
} {
	t.Helper()
	var rows []struct {
		WarehouseID string
		Quantity    int
		ShipmentID  *string
	}
	require.NoError(t, db.Raw(
		`SELECT warehouse_id, sum(quantity) AS quantity, max(shipment_id::text) AS shipment_id
		   FROM order_allocations WHERE order_id = ?
		  GROUP BY warehouse_id ORDER BY warehouse_id`, orderID).Scan(&rows).Error)
	return rows
}

func shipmentsForOrder(t *testing.T, db *gorm.DB, orderID string) []struct {
	ID          string
	WarehouseID *string
} {
	t.Helper()
	var rows []struct {
		ID          string
		WarehouseID *string
	}
	require.NoError(t, db.Raw(
		`SELECT id, warehouse_id::text AS warehouse_id FROM shipments
		  WHERE order_id = ? ORDER BY created_at, id`, orderID).Scan(&rows).Error)
	return rows
}

// shipFromLine1 reads ship_from->>'line1' for a shipment — used to prove
// two shipments actually shipped from DIFFERENT pickup addresses, not just
// that two shipment rows exist.
func shipFromLine1(t *testing.T, db *gorm.DB, shipmentID string) string {
	t.Helper()
	var line1 string
	require.NoError(t, db.Raw(
		`SELECT ship_from->>'line1' FROM shipments WHERE id = ?`, shipmentID).
		Row().Scan(&line1))
	return line1
}

// ─── stub carrier ───────────────────────────────────────────────────────

// stubCarrier implements shipping.Carrier without any network call. Its
// CreateShipment response encodes the pickup address it was given into the
// tracking number, so a test can prove a parcel shipped from the RIGHT
// warehouse — not merely that some parcel was created. Shipping from the
// wrong warehouse is the failure #177 exists to prevent.
//
// failOnceForLine1, when set, makes the FIRST CreateShipment call for that
// exact pickup Line1 fail, then succeed on every call after — simulating a
// carrier failure part-way through a multi-warehouse Create() and the
// retry that follows.
type stubCarrier struct {
	failOnceForLine1 string
	failed           atomic.Bool

	// mu guards calls — createShipmentsPerWarehouse calls CreateShipment
	// sequentially within one Create() request, but a test may hold onto
	// this stub across the request, so guard it anyway.
	mu    sync.Mutex
	calls []shipping.ShipmentRequest
}

func (s *stubCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) {
	return nil, fmt.Errorf("stubCarrier: GetRates not implemented")
}

func (s *stubCarrier) CreateShipment(_ context.Context, in shipping.ShipmentRequest) (*shipping.Shipment, error) {
	if s.failOnceForLine1 != "" && in.FromAddress.Line1 == s.failOnceForLine1 && s.failed.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("stubCarrier: simulated carrier failure for %s", in.FromAddress.Line1)
	}
	s.mu.Lock()
	s.calls = append(s.calls, in)
	s.mu.Unlock()
	return &shipping.Shipment{
		ProviderShipmentID: "PSID-" + in.FromAddress.Line1,
		TrackingNumber:     "TRK-" + in.FromAddress.Line1,
		Service:            in.Service,
	}, nil
}

func (s *stubCarrier) GetTracking(context.Context, string) (*shipping.Tracking, error) {
	return nil, fmt.Errorf("stubCarrier: GetTracking not implemented")
}

func (s *stubCarrier) CancelShipment(context.Context, string) error {
	return fmt.Errorf("stubCarrier: CancelShipment not implemented")
}

func (s *stubCarrier) ProviderName() string         { return "stubcarrier" }
func (s *stubCarrier) SupportedCountries() []string { return []string{"IN"} }

// ─── fixtures ────────────────────────────────────────────────────────────

var perWarehouseTables = []string{
	"order_allocations",
	"shipments",
	"order_events",
	"outbox_events",
	"order_addresses",
	"order_items",
	"orders",
	"shipping_carrier_configs",
	"warehouses",
	"stores",
}

// newPerWarehouseHandler wires a real order.Service (backed by the same
// db) so createShipmentsPerWarehouse's fulfilment-status reporting is
// exercised end-to-end, not skipped as it would be if orderSvc were left
// nil — see TestCreateShipment_* fulfillmentStatusForOrder assertions
// below, which are what step 6 of the task-2 brief mutation-tests.
func newPerWarehouseHandler(db *gorm.DB, carrier shipping.Carrier) *ShipmentsHandler {
	repo := shipping.NewRepository(db)
	svc := shipping.NewShippingService(repo)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orderRepo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	orderSvc := order.NewService(db, orderRepo, outboxRepo)
	h := NewShipmentsHandler(db, svc, repo, nil, logger).WithOrderService(orderSvc)
	h.WithCarrierConstructor(func(provider, apiKey, secretKey, mode string) (shipping.Carrier, error) {
		return carrier, nil
	})
	return h
}

// fulfillmentStatusForOrder reads orders.fulfillment_status directly —
// the property createShipmentsPerWarehouse's remaining-groups decision
// (brief step 4) is responsible for.
func fulfillmentStatusForOrder(t *testing.T, db *gorm.DB, orderID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, db.Raw(
		`SELECT fulfillment_status FROM orders WHERE id = ?`, orderID).Row().Scan(&status))
	return status
}

// orderStatusForOrder reads orders.status directly — the axis label
// creation must NEVER touch (task-2 FIX 1). Label creation only ever
// advances fulfillment_status; orders.status is the manual /fulfill admin
// endpoint's job.
func orderStatusForOrder(t *testing.T, db *gorm.DB, orderID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, db.Raw(
		`SELECT status FROM orders WHERE id = ?`, orderID).Row().Scan(&status))
	return status
}

func seedPerWarehouseStore(t *testing.T, db *gorm.DB) (storeID, tenantID uuid.UUID) {
	t.Helper()
	tenantID, storeID = uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	return storeID, tenantID
}

func seedPerWarehouseWarehouse(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID, name, line1 string) warehouse.Warehouse {
	t.Helper()
	repo := warehouse.NewRepository()
	wh, err := repo.Upsert(context.Background(), db, warehouse.Warehouse{
		TenantID: tenantID.String(), StoreID: storeID.String(), Name: name,
		Line1: line1, City: "Mumbai", Region: "MH",
		PostalCode: "400001", CountryCode: "IN", Phone: "+912200000000",
		ContactPerson: "Warehouse Manager",
	})
	require.NoError(t, err)
	return wh
}

func seedPerWarehouseCarrierConfig(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID, whID *uuid.UUID) *shipping.CarrierConfig {
	t.Helper()
	cfg := &shipping.CarrierConfig{
		TenantID:    tenantID,
		StoreID:     storeID,
		Provider:    "stubcarrier",
		APIKey:      "test-key",
		Mode:        "test",
		Enabled:     true,
		HandlingFee: decimal.Zero,
		WarehouseID: whID,
	}
	require.NoError(t, db.Create(cfg).Error)
	return cfg
}

func seedPerWarehouseOrder(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	return seedPerWarehouseOrderWithStatus(t, db, storeID, tenantID, "confirmed")
}

// seedPerWarehouseOrderWithStatus is seedPerWarehouseOrder with the
// orders.status column exposed — used to cover the "order still pending
// when parcels ship" fulfilment-status-can't-report case, which requires
// an order Create() will accept (Create has no order-status precondition)
// but MarkFulfilled's confirmed->fulfilled guard will reject.
func seedPerWarehouseOrderWithStatus(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	o := &order.Order{
		TenantID:       tenantID,
		StoreID:        storeID,
		OrderNumber:    "M-" + uuid.NewString()[:8],
		IdempotencyKey: "idem-" + uuid.NewString(),
		CustomerEmail:  "buyer@example.com",
		Status:         status,
		PaymentStatus:  "paid",
		Subtotal:       decimal.NewFromInt(1000),
		GrandTotal:     decimal.NewFromInt(1000),
		CurrencyCode:   "INR",
	}
	require.NoError(t, db.Create(o).Error)

	addr := &order.OrderAddress{
		OrderID:     o.ID,
		Kind:        "shipping",
		Name:        "Jane Doe",
		Line1:       "42 Example Lane",
		City:        "Mumbai",
		CountryCode: "IN",
	}
	require.NoError(t, db.Create(addr).Error)

	return o.ID
}

func seedPerWarehouseItem(t *testing.T, db *gorm.DB, orderID uuid.UUID, sku string, qty int) uuid.UUID {
	t.Helper()
	it := &order.OrderItem{
		OrderID:       orderID,
		TitleSnapshot: "Test Widget " + sku,
		SKUSnapshot:   sku,
		UnitPrice:     decimal.NewFromInt(500),
		Quantity:      qty,
		LineTotal:     decimal.NewFromInt(500 * int64(qty)),
		CurrencyCode:  "INR",
	}
	require.NoError(t, db.Create(it).Error)
	return it.ID
}

func seedAllocation(t *testing.T, db *gorm.DB, tenantID, storeID, orderID, itemID uuid.UUID, warehouseID string, qty int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO order_allocations (tenant_id, store_id, order_id, order_item_id, warehouse_id, quantity)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tenantID, storeID, orderID, itemID, warehouseID, qty).Error)
}

// createShipmentViaHandler calls Create() directly against a hand-built
// gin.Context — the same lightweight pattern shipments_cancel_test.go
// already uses in this package — rather than standing up the full
// RegisterAdmin router with auth/tenant middleware, which Create() itself
// does not depend on.
func createShipmentViaHandler(t *testing.T, h *ShipmentsHandler, storeID, orderID, tenantID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body, err := json.Marshal(CreateShipmentRequest{Provider: "stubcarrier", Service: "standard"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "storeId", Value: storeID.String()},
		{Key: "id", Value: orderID.String()},
	}
	c.Set("tenant_id", tenantID.String())
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))

	h.Create(c)
	return w
}

// ─── tests ───────────────────────────────────────────────────────────────

// TestCreateShipment_OrderWithNoAllocationsCreatesOneShipmentAsBefore is
// the load-bearing case: every order in production today has no
// order_allocations rows, so this is the ONLY path currently executing.
// It must produce exactly one shipment, pickup from the carrier config's
// warehouse, warehouse_id left NULL — unchanged from before #177 PR 4b.
func TestCreateShipment_OrderWithNoAllocationsCreatesOneShipmentAsBefore(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	wh := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Main Warehouse", "1 Main Warehouse Road")
	whUUID := uuid.MustParse(wh.ID)
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, &whUUID)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	seedPerWarehouseItem(t, db, orderID, "SKU-1", 2)
	// Deliberately NO order_allocations rows.

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := createShipmentViaHandler(t, h, storeID, orderID, tenantID)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 1, "an order with no allocations must produce exactly one shipment")
	require.Nil(t, shipments[0].WarehouseID, "with no allocation to attribute it to, warehouse_id must stay NULL")

	// The no-allocations path keeps whatever behaviour it had before #177
	// PR 4b: createSingleShipment has never reported fulfilment status, and
	// this task must not start it doing so.
	require.Equal(t, "unfulfilled", fulfillmentStatusForOrder(t, db, orderID),
		"the single-shipment path must not change fulfilment status")
}

// TestCreateShipment_OneAllocationGroupCreatesOneShipmentWithItsWarehouse
// covers a single warehouse with allocations present: one shipment,
// carrying that warehouse's id, and the allocation rows stamped with the
// resulting shipment's id.
func TestCreateShipment_OneAllocationGroupCreatesOneShipmentWithItsWarehouse(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	wh := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	itemID := seedPerWarehouseItem(t, db, orderID, "SKU-1", 3)
	seedAllocation(t, db, tenantID, storeID, orderID, itemID, wh.ID, 3)

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := createShipmentViaHandler(t, h, storeID, orderID, tenantID)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 1)
	require.NotNil(t, shipments[0].WarehouseID)
	require.Equal(t, wh.ID, *shipments[0].WarehouseID)

	groups := groupsForOrder(t, db, orderID.String())
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0].ShipmentID, "the allocation row must be stamped with the shipment it went out on")
	require.Equal(t, shipments[0].ID, *groups[0].ShipmentID)

	// Every group this order owed a parcel for now has one — fulfilled,
	// not partial.
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, db, orderID))

	// task-2 FIX 1: label creation must move ONLY fulfillment_status.
	// orders.status stays exactly where it started (confirmed, per
	// seedPerWarehouseOrder) — orders.status="fulfilled" is TERMINAL
	// (status.go), so if this ever regresses to calling MarkFulfilled here,
	// the order becomes permanently un-cancellable the moment its first
	// (and, for a single-warehouse store, only) label is created — before
	// the parcel is even picked up.
	require.Equal(t, "confirmed", orderStatusForOrder(t, db, orderID),
		"orders.status must be untouched by label creation, not advanced to fulfilled")
}

// TestCreateShipment_TwoAllocationGroupsCreateTwoShipments is the core
// property: two warehouses produce two shipments, each carrying its OWN
// warehouse_id, and — critically — shipping from DIFFERENT ship_from
// addresses. Shipping from the wrong warehouse is the failure #177 exists
// to prevent, so two shipment rows alone would not be enough to trust.
func TestCreateShipment_TwoAllocationGroupsCreateTwoShipments(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	whA := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	whB := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse B", "2 Warehouse B Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	itemA := seedPerWarehouseItem(t, db, orderID, "SKU-A", 2)
	itemB := seedPerWarehouseItem(t, db, orderID, "SKU-B", 5)
	seedAllocation(t, db, tenantID, storeID, orderID, itemA, whA.ID, 2)
	seedAllocation(t, db, tenantID, storeID, orderID, itemB, whB.ID, 5)

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := createShipmentViaHandler(t, h, storeID, orderID, tenantID)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 2)

	gotWarehouses := map[string]string{} // warehouse_id -> shipment id
	for _, s := range shipments {
		require.NotNil(t, s.WarehouseID)
		gotWarehouses[*s.WarehouseID] = s.ID
	}
	require.Contains(t, gotWarehouses, whA.ID)
	require.Contains(t, gotWarehouses, whB.ID)

	line1A := shipFromLine1(t, db, gotWarehouses[whA.ID])
	line1B := shipFromLine1(t, db, gotWarehouses[whB.ID])
	require.Equal(t, "1 Warehouse A Road", line1A)
	require.Equal(t, "2 Warehouse B Road", line1B)
	require.NotEqual(t, line1A, line1B, "the two shipments must ship from DIFFERENT pickup addresses")

	groups := groupsForOrder(t, db, orderID.String())
	require.Len(t, groups, 2)
	for _, g := range groups {
		require.NotNil(t, g.ShipmentID)
		require.Equal(t, gotWarehouses[g.WarehouseID], *g.ShipmentID)
	}

	// Both groups shipped in this one call — the order is fully fulfilled.
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, db, orderID))

	// orders.status is a separate axis label creation never touches.
	require.Equal(t, "confirmed", orderStatusForOrder(t, db, orderID))
}

// TestCreateShipment_AlreadyShippedGroupIsNotShippedTwice pins the
// retry-after-partial-failure property. Warehouse A's parcel succeeds on
// the first Create() call; warehouse B's carrier call fails, so Create
// aborts having created exactly one shipment. A second Create() call (the
// retry) must NOT re-ship A — its allocation already carries a
// shipment_id — and must create exactly one new shipment, for B.
func TestCreateShipment_AlreadyShippedGroupIsNotShippedTwice(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	whA := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	whB := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse B", "2 Warehouse B Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	itemA := seedPerWarehouseItem(t, db, orderID, "SKU-A", 1)
	itemB := seedPerWarehouseItem(t, db, orderID, "SKU-B", 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemA, whA.ID, 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemB, whB.ID, 1)

	// Groups are walked in warehouse_id order, so fail whichever of A/B
	// sorts second — the FIRST call in that order must succeed and
	// persist before the failing one aborts the request.
	failLine1 := "2 Warehouse B Road"
	if whB.ID < whA.ID {
		failLine1 = "1 Warehouse A Road"
	}
	carrier := &stubCarrier{failOnceForLine1: failLine1}
	h := newPerWarehouseHandler(db, carrier)

	// First call: one group succeeds, the other fails the whole request.
	w1 := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusBadGateway, w1.Code, w1.Body.String())

	shipmentsAfterFirst := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipmentsAfterFirst, 1, "the succeeding group's shipment must be persisted despite the other group's failure")

	// One of two groups has a real, persisted parcel; the other still owes
	// one. The order must report partial — not fulfilled (that would tell
	// a customer their order is complete while a parcel is missing) and
	// not unfulfilled (that would hide the parcel that already shipped).
	require.Equal(t, "partial", fulfillmentStatusForOrder(t, db, orderID),
		"a two-group order with only one parcel shipped must report partial fulfilment")

	// Second call: the retry. The already-shipped group must not be
	// re-shipped; only the previously-failed group gets a new shipment.
	w2 := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusCreated, w2.Code, w2.Body.String())

	shipmentsAfterRetry := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipmentsAfterRetry, 2, "the retry must add exactly one shipment, for the group that had failed")

	// Now every group owed a parcel has one.
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, db, orderID))

	firstIDs := map[string]bool{}
	for _, s := range shipmentsAfterFirst {
		firstIDs[s.ID] = true
	}
	newCount := 0
	for _, s := range shipmentsAfterRetry {
		if !firstIDs[s.ID] {
			newCount++
		}
	}
	require.Equal(t, 1, newCount, "the retry must not touch the shipment already created for the succeeding group")

	groups := groupsForOrder(t, db, orderID.String())
	require.Len(t, groups, 2)
	for _, g := range groups {
		require.NotNil(t, g.ShipmentID, "both groups must be stamped after the retry")
	}
}

// TestCreateShipment_FullyShippedOrderReCallIsNoOp pins CRITICAL 1 from
// fix round 1: an order whose allocations are ALL already shipped is a
// different state from "no allocations at all" — len(groups)==0 alone
// cannot tell them apart. A re-POST on a fully-shipped order must be a
// 409 no-op, not fall through to createSingleShipment and buy a second,
// real, un-cancellable label for the whole order.
func TestCreateShipment_FullyShippedOrderReCallIsNoOp(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	wh := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	itemID := seedPerWarehouseItem(t, db, orderID, "SKU-1", 2)
	seedAllocation(t, db, tenantID, storeID, orderID, itemID, wh.ID, 2)

	h := newPerWarehouseHandler(db, &stubCarrier{})

	w1 := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusCreated, w1.Code, w1.Body.String())

	shipmentsAfterFirst := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipmentsAfterFirst, 1)
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, db, orderID))

	// Every allocation is now shipped. A second Create() call must be a
	// no-op: 409, and NOT a second whole-order shipment via
	// createSingleShipment falling through on len(groups)==0.
	w2 := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusConflict, w2.Code, w2.Body.String())

	shipmentsAfterRecall := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipmentsAfterRecall, 1, "a re-POST on a fully-shipped order must not create another shipment")
	require.Equal(t, shipmentsAfterFirst[0].ID, shipmentsAfterRecall[0].ID)

	// The no-op recall must not touch fulfilment status either — it never
	// reaches createShipmentsPerWarehouse's per-group reporting.
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, db, orderID))
}

// TestCreateShipment_PendingOrderShipsButFulfilmentStatusReportedAsWarning
// covers fix-round-1 IMPORTANT 1: Create() has no order-status
// precondition, and the admin UI explicitly allows labelling a `pending`
// order — but MarkFulfilled requires orders.status == confirmed, and
// MarkPartiallyFulfilled's guard is on the fulfillment axis only (no order
// axis check). So on a two-group pending order, the first group's
// MarkPartiallyFulfilled call succeeds (no order-status guard fires), then
// the second group recomputes remaining==0 and calls MarkFulfilled, which
// fails InvalidTransition because orders.status is still "pending".
//
// Both parcels must exist at the carrier regardless — the failure must
// NOT turn into a 5xx that would suggest the shipment itself failed. The
// order is left stuck on fulfillment_status=partial (never advanced to
// fulfilled), and that must be surfaced to the admin via the response's
// pickup-warning channel, not just logged.
// TestCreateShipment_PendingOrderShipsAndFulfilmentStatusStillAdvances pins
// task-2 FIX 1's actual mechanism: fulfillment_status is reported via
// MarkFulfillmentComplete/MarkPartiallyFulfilled, which have ONLY a
// fulfillment_status precondition — never an orders.status one. Before the
// fix, the last group's report went through MarkFulfilled, which also
// requires orders.status to legally reach "fulfilled" (confirmed only);
// on a still-pending order that precondition failed, silently stranding
// fulfillment_status at "partial" even though every parcel had shipped.
// Post-fix, both parcels ship AND fulfillment_status correctly reaches
// "fulfilled" regardless of orders.status — which itself is never touched.
func TestCreateShipment_PendingOrderShipsAndFulfilmentStatusStillAdvances(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	whA := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	whB := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse B", "2 Warehouse B Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrderWithStatus(t, db, storeID, tenantID, "pending")
	itemA := seedPerWarehouseItem(t, db, orderID, "SKU-A", 1)
	itemB := seedPerWarehouseItem(t, db, orderID, "SKU-B", 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemA, whA.ID, 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemB, whB.ID, 1)

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := createShipmentViaHandler(t, h, storeID, orderID, tenantID)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 2)

	groups := groupsForOrder(t, db, orderID.String())
	require.Len(t, groups, 2)
	for _, g := range groups {
		require.NotNil(t, g.ShipmentID, "both allocation groups must be stamped shipped")
	}

	// fulfillment_status reaches "fulfilled" — no longer gated on
	// orders.status, which a pending order would have failed under the
	// old MarkFulfilled call.
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, db, orderID))

	// orders.status itself must be untouched — still pending, never
	// silently bumped by label creation.
	require.Equal(t, "pending", orderStatusForOrder(t, db, orderID))

	var resp ShipmentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.PickupWarning, "fulfilment-status reporting must succeed, not warn, once it no longer depends on orders.status")
}

// TestCreateShipment_VariantlessItemIsIncludedInFirstShipmentsParcel pins
// task-2 FIX 2: stockLinesFromItems (checkout_stock.go) deliberately skips
// order items with a nil variant_id when computing allocations — a
// custom/unstocked line is a supported order shape. Such an item never
// gets an order_allocations row, so it never lands in any allocation
// group's ItemIDs. Before the fix, createShipmentsPerWarehouse built each
// group's parcel from ONLY that group's ItemIDs, so the variantless item
// would silently vanish from every shipment — the order gets marked
// fulfilled having shipped an incomplete parcel. It must instead ride
// along in the first (highest-priority) group's parcel. This single-call,
// single-group order can't distinguish "first group of this call" from
// "first shipment overall" — see
// TestCreateShipment_UnallocatedItemShipsOnlyOnceAcrossRetry below for the
// case that does.
func TestCreateShipment_VariantlessItemIsIncludedInFirstShipmentsParcel(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	wh := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)

	// A stocked line — allocated to Warehouse A, so it drives the one
	// allocation group this order has.
	stockedID := seedPerWarehouseItem(t, db, orderID, "SKU-STOCKED", 2)
	seedAllocation(t, db, tenantID, storeID, orderID, stockedID, wh.ID, 2)

	// A custom/unstocked line with NO variant_id and, critically, NO
	// order_allocations row at all — nothing "stockLinesFromItems" ever
	// wrote for it. seedPerWarehouseItem never sets VariantID, so it is
	// nil by default, matching this order shape exactly.
	seedPerWarehouseItem(t, db, orderID, "SKU-CUSTOM", 1)

	carrier := &stubCarrier{}
	h := newPerWarehouseHandler(db, carrier)
	w := createShipmentViaHandler(t, h, storeID, orderID, tenantID)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 1, "one allocation group means one shipment — the unallocated item must not create a second one")

	require.Len(t, carrier.calls, 1)
	skus := make(map[string]int, 2)
	for _, item := range carrier.calls[0].Items {
		skus[item.SKU] = item.Quantity
	}
	require.Equal(t, 2, skus["SKU-STOCKED"], "the allocated item must still ship at its allocated quantity")
	require.Equal(t, 1, skus["SKU-CUSTOM"], "the variantless item must ride along in the parcel at its full order quantity, not vanish")

	// The unallocated item has no order_allocations row, so it stays
	// invisible to groupsForOrder — only the stocked line's allocation is
	// there, and it must be stamped shipped.
	groups := groupsForOrder(t, db, orderID.String())
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0].ShipmentID)
}

// TestCreateShipment_UnallocatedItemShipsOnlyOnceAcrossRetry is the
// regression test for the double-ship bug: an unallocated (variantless)
// item riding along in "the first group's parcel" must be gated on this
// being the order's first shipment EVER, not on gi==0 within one call.
//
// groups comes from unshippedAllocationGroups, which is recomputed
// REMAINING-only on every call. Sequence:
//  1. Call 1: groups = [A, B]. A is gi==0, so the variantless item rides in
//     A's parcel. A's label succeeds and persists.
//  2. B's carrier call fails; Create() aborts having created exactly one
//     shipment (for A), leaving B's allocation unshipped.
//  3. Retry: groups = [B] alone (A no longer owes a parcel). B is now
//     gi==0 for THIS call — a positional check would append the
//     variantless item to B's parcel too, shipping it a second time. The
//     item has no allocation row, so nothing would ever catch the
//     duplicate: it just goes out twice, for real, at a carrier that
//     can't un-create a label.
//
// Gating on "does this order already have any shipments rows" instead
// stays correct across the retry: it was true only on call 1's group A.
func TestCreateShipment_UnallocatedItemShipsOnlyOnceAcrossRetry(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	whA := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	whB := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse B", "2 Warehouse B Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	itemA := seedPerWarehouseItem(t, db, orderID, "SKU-A", 1)
	itemB := seedPerWarehouseItem(t, db, orderID, "SKU-B", 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemA, whA.ID, 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemB, whB.ID, 1)

	// The variantless line: no VariantID, no order_allocations row at all.
	seedPerWarehouseItem(t, db, orderID, "SKU-CUSTOM", 1)

	// Groups are walked in warehouse_id order. Fail whichever of A/B sorts
	// SECOND, so the first call's gi==0 group (which gets the variantless
	// item, correctly, under both the old and new code) is the one that
	// succeeds and persists — and the failing group is the one left for
	// the retry to pick up as its OWN gi==0.
	failLine1 := "2 Warehouse B Road"
	if whB.ID < whA.ID {
		failLine1 = "1 Warehouse A Road"
	}
	carrier := &stubCarrier{failOnceForLine1: failLine1}
	h := newPerWarehouseHandler(db, carrier)

	// First call: one group succeeds (carrying the variantless item),
	// the other fails the whole request.
	w1 := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusBadGateway, w1.Code, w1.Body.String())
	require.Len(t, carrier.calls, 1, "exactly one parcel must have been sent to the carrier before the failure")

	// Retry: only the previously-failed group remains, and it is gi==0 for
	// THIS call.
	w2 := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusCreated, w2.Code, w2.Body.String())

	require.Len(t, carrier.calls, 2, "the retry must add exactly one more parcel, for the previously-failed group")

	// The load-bearing assertion: SKU-CUSTOM must appear in EXACTLY ONE of
	// the two parcels sent to the carrier across both calls — never zero
	// (it must still ship), never two (it must not ship twice).
	parcelsContainingCustom := 0
	for _, call := range carrier.calls {
		for _, item := range call.Items {
			if item.SKU == "SKU-CUSTOM" {
				parcelsContainingCustom++
				require.Equal(t, 1, item.Quantity)
			}
		}
	}
	require.Equal(t, 1, parcelsContainingCustom,
		"the unallocated item must ship in exactly one parcel across the call and its retry, not zero and not two")

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 2, "one shipment per allocation group, across both calls")
}
