//go:build integration

// Package storefront — the seam between allocation and fulfilment (#177).
//
// Both halves of the split-shipment story were already covered, and both
// passed:
//
//   - checkout_allocation_integration_test.go proves commitStock writes two
//     order_allocations rows when one line draws from two warehouses.
//   - handlers/admin/shipments_per_warehouse_integration_test.go proves two
//     allocation groups produce two shipments.
//
// Nothing joined them. That second suite calls seedAllocation(), which
// INSERTs order_allocations rows by hand — so every assertion about
// shipments was made against allocations a test author wrote, never against
// allocations the allocator produced. If the two ever disagreed about
// column types, grouping, casing or quantity attribution, both suites would
// stay green and production would still ship one parcel from the wrong
// warehouse.
//
// That gap is a seam, and seams are where this system's real bugs have
// been: the sentinel that made allocation economically hollow, the dialog
// scrim that was an animation-fill-mode interacting with a position:fixed
// three components away. Neither component was wrong alone.
//
// This test owns the join and nothing else. It runs the real allocator,
// then hands its output — untouched — to the real shipment handler.
//
// It deliberately does NOT need carrier credentials. Create() takes a
// carrier through WithCarrierConstructor, so the storefront-checkout
// blocker (#494, no ShipEngine config for bondi) does not apply here.
package storefront

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	adminh "github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ─── a carrier that records where each parcel was picked up from ────────

// seamCarrier encodes the pickup address into the tracking number, so the
// test can prove each parcel shipped FROM THE RIGHT WAREHOUSE rather than
// merely that two shipment rows appeared. Shipping both parcels from one
// origin is the exact failure #177 exists to prevent, and it would satisfy
// a naive "two shipments were created" assertion.
type seamCarrier struct {
	pickups []string
}

func (c *seamCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) {
	return nil, fmt.Errorf("seamCarrier: GetRates not used")
}

func (c *seamCarrier) CreateShipment(_ context.Context, in shipping.ShipmentRequest) (*shipping.Shipment, error) {
	c.pickups = append(c.pickups, in.FromAddress.Line1)
	return &shipping.Shipment{
		ProviderShipmentID: "PSID-" + in.FromAddress.Line1,
		TrackingNumber:     "TRK-" + in.FromAddress.Line1,
		Service:            in.Service,
	}, nil
}

func (c *seamCarrier) GetTracking(context.Context, string) (*shipping.Tracking, error) {
	return nil, fmt.Errorf("seamCarrier: GetTracking not used")
}
func (c *seamCarrier) CancelShipment(context.Context, string) error {
	return fmt.Errorf("seamCarrier: CancelShipment not used")
}
func (c *seamCarrier) ProviderName() string         { return "stubcarrier" }
func (c *seamCarrier) SupportedCountries() []string { return []string{"IN"} }

// ─── fixtures the shipment half needs on top of the allocation half ─────

// makeOrderShippable gives an order what Create() requires and
// seedOrderWithItems (written for allocation-only tests) does not set: a
// confirmed/paid order and a shipping address.
func makeOrderShippable(t *testing.T, db *gorm.DB, orderID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`UPDATE orders SET status = 'confirmed', payment_status = 'paid' WHERE id = ?`,
		orderID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO order_addresses (order_id, kind, name, line1, city, country_code)
		 VALUES (?, 'shipping', 'Jane Doe', '42 Example Lane', 'Mumbai', 'IN')`,
		orderID).Error)
}

func seedSeamCarrierConfig(t *testing.T, db *gorm.DB, storeID, tenantID string) {
	t.Helper()
	sID, err := uuid.Parse(storeID)
	require.NoError(t, err)
	tID, err := uuid.Parse(tenantID)
	require.NoError(t, err)
	require.NoError(t, db.Create(&shipping.CarrierConfig{
		TenantID: tID, StoreID: sID, Provider: "stubcarrier",
		APIKey: "test-key", Mode: "test", Enabled: true,
		HandlingFee: decimal.Zero,
	}).Error)
}

func newSeamShipmentsHandler(db *gorm.DB, carrier shipping.Carrier) *adminh.ShipmentsHandler {
	repo := shipping.NewRepository(db)
	svc := shipping.NewShippingService(repo)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orderSvc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	return adminh.NewShipmentsHandler(db, svc, repo, nil, logger).
		WithOrderService(orderSvc).
		WithCarrierConstructor(func(_, _, _, _ string) (shipping.Carrier, error) {
			return carrier, nil
		})
}

func createShipmentsForOrder(t *testing.T, h *adminh.ShipmentsHandler, storeID, orderID, tenantID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body, err := json.Marshal(map[string]string{"provider": "stubcarrier", "service": "standard"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "storeId", Value: storeID},
		{Key: "id", Value: orderID},
	}
	c.Set("tenant_id", tenantID)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	h.Create(c)
	return w
}

// testdbForSeam truncates the union of both halves' tables: the allocation
// side (stock, holds, allocations) and the fulfilment side (shipments,
// events, carrier configs). Order matters — children before parents.
func testdbForSeam(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.NewDB(t,
		"shipments",
		"order_allocations",
		"stock_holds",
		"order_events",
		"outbox_events",
		"order_addresses",
		"order_items",
		"orders",
		"shipping_carrier_configs",
		"variant_stock",
		"product_variants",
		"products",
		"warehouses",
		"stores",
	)
}

func tenantOf(t *testing.T, db *gorm.DB, storeID string) string {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))
	return tenantID
}

// ─── the seam ───────────────────────────────────────────────────────────

// TestSeam_RealAllocationsProduceOneShipmentPerWarehouse is the end-to-end
// proof #177 never had: ONE order, allocated by the real allocator across
// TWO warehouses, fulfilled by the real shipment handler into TWO parcels
// picked up from two different addresses.
//
// Every assertion below reads rows that the production code wrote. Nothing
// hand-seeds order_allocations.
func TestSeam_RealAllocationsProduceOneShipmentPerWarehouse(t *testing.T) {
	db := testdbForSeam(t)

	storeID, variantID := seedAvailStore(t, db)
	tenantID := tenantOf(t, db, storeID)

	// Two warehouses with DIFFERENT pickup addresses, so a parcel can be
	// traced back to the warehouse that filled it.
	whA := seedWarehouseRow(t, db, storeID, "A")
	whB := seedWarehouseRow(t, db, storeID, "B")
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 0, line1 = '1 Alpha Road' WHERE id = ?`, whA).Error)
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 1, line1 = '2 Bravo Street' WHERE id = ?`, whB).Error)

	// 3 units at A, 4 at B. An order for 5 must draw from both.
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, whA, variantID, whB).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 5}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	// ── half one: the REAL allocator ──────────────────────────────────
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	var allocs []struct {
		WarehouseID string
		Quantity    int
	}
	require.NoError(t, db.Raw(
		`SELECT warehouse_id, quantity FROM order_allocations
		  WHERE order_item_id = ? ORDER BY quantity DESC`, itemIDs[0]).Scan(&allocs).Error)
	require.Len(t, allocs, 2, "precondition: 5 units over a 3+4 split must allocate across both warehouses")

	// ── half two: the REAL shipment handler, over those exact rows ────
	makeOrderShippable(t, db, orderID)
	seedSeamCarrierConfig(t, db, storeID, tenantID)

	carrier := &seamCarrier{}
	h := newSeamShipmentsHandler(db, carrier)
	w := createShipmentsForOrder(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	// ── the join ──────────────────────────────────────────────────────
	var shipments []struct {
		ID          string
		WarehouseID *string
	}
	require.NoError(t, db.Raw(
		`SELECT id, warehouse_id::text AS warehouse_id FROM shipments
		  WHERE order_id = ? ORDER BY created_at, id`, orderID).Scan(&shipments).Error)

	require.Len(t, shipments, 2,
		"two allocation groups written by the allocator must produce two shipments")

	// Each shipment must name one of the two warehouses, and they must be
	// different ones. A pair that both point at the default warehouse is
	// the regression this test exists to catch.
	got := map[string]bool{}
	for _, s := range shipments {
		require.NotNil(t, s.WarehouseID, "a shipment from an allocated order must name its warehouse")
		got[*s.WarehouseID] = true
	}
	require.Len(t, got, 2, "the two shipments must come from two DIFFERENT warehouses, got %v", got)
	require.True(t, got[whA], "no shipment was attributed to warehouse A")
	require.True(t, got[whB], "no shipment was attributed to warehouse B")

	// And the carrier must have been asked to pick up from both addresses
	// — proving the pickup address travelled with the allocation rather
	// than defaulting to the carrier config's warehouse for both parcels.
	require.ElementsMatch(t, []string{"1 Alpha Road", "2 Bravo Street"}, carrier.pickups,
		"each parcel must be picked up from the warehouse that filled it")
}

// TestSeam_SingleWarehouseAllocationStillProducesOneShipment is the
// control. The interesting assertion above is "two", and a bug that made
// createShipmentsPerWarehouse emit one shipment per allocation ROW rather
// than per warehouse GROUP would also produce two here — from a single
// warehouse that happens to fill two lines. Without this case, that bug
// reads as a pass.
func TestSeam_SingleWarehouseAllocationStillProducesOneShipment(t *testing.T) {
	db := testdbForSeam(t)

	storeID, variantID := seedAvailStore(t, db)
	tenantID := tenantOf(t, db, storeID)

	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 0, line1 = '1 Alpha Road' WHERE id = ?`, whA).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 10, now())`, variantID, whA).Error)

	// TWO lines of the same variant, both fillable from the one warehouse.
	lines := []stockLine{{VariantID: variantID, Quantity: 2}, {VariantID: variantID, Quantity: 3}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	var allocCount int
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM order_allocations WHERE order_id = ?`, orderID).Row().Scan(&allocCount))
	require.Equal(t, 2, allocCount,
		"precondition: two lines allocate as two rows, both at the same warehouse")

	makeOrderShippable(t, db, orderID)
	seedSeamCarrierConfig(t, db, storeID, tenantID)

	carrier := &seamCarrier{}
	h := newSeamShipmentsHandler(db, carrier)
	w := createShipmentsForOrder(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	var shipmentCount int
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM shipments WHERE order_id = ?`, orderID).Row().Scan(&shipmentCount))
	require.Equal(t, 1, shipmentCount,
		"two allocation ROWS at ONE warehouse are one group, and must ship as one parcel")
	require.Len(t, carrier.pickups, 1)
}
