//go:build integration

// Package admin — coverage for #496: the admin surface only ever showed and
// let you operate one parcel of a multi-warehouse order.
//
// GetShipmentByOrderID's First() with no ORDER BY returned an arbitrary
// parcel (effectively by primary key), and UpdateStatus resolved the record
// via that same call before checking rec.ID == :shipmentId — so the second
// parcel on a two-warehouse order 404'd on every manual status update. This
// file proves: (1) GetByOrder?all=true returns both parcels, oldest first;
// (2) GetByOrder with no param deterministically returns the FIRST parcel;
// (3) UpdateStatus now succeeds for the SECOND parcel; (4) a shipment id
// belonging to a different order or store still 404s.
//
// It reuses the per-warehouse test harness (stubCarrier, newPerWarehouseHandler,
// seed helpers) from shipments_per_warehouse_integration_test.go to build a
// real two-parcel order via Create(), rather than hand-inserting shipment
// rows — that keeps these tests honest about what production actually
// produces.
package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedTwoParcelOrder builds a confirmed order allocated across two
// warehouses and ships it via Create(), producing two real shipments rows.
// Returns the store/tenant/order ids and the two shipment ids in creation
// order (first, second).
func seedTwoParcelOrder(t *testing.T, db *gorm.DB) (storeID, tenantID, orderID uuid.UUID, firstShipmentID, secondShipmentID string) {
	t.Helper()
	storeID, tenantID = seedPerWarehouseStore(t, db)
	whA := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse A", "1 Warehouse A Road")
	whB := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Warehouse B", "2 Warehouse B Road")
	seedPerWarehouseCarrierConfig(t, db, storeID, tenantID, nil)
	orderID = seedPerWarehouseOrder(t, db, storeID, tenantID)
	itemA := seedPerWarehouseItem(t, db, orderID, "SKU-A", 1)
	itemB := seedPerWarehouseItem(t, db, orderID, "SKU-B", 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemA, whA.ID, 1)
	seedAllocation(t, db, tenantID, storeID, orderID, itemB, whB.ID, 1)

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := createShipmentViaHandler(t, h, storeID, orderID, tenantID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	shipments := shipmentsForOrder(t, db, orderID.String())
	require.Len(t, shipments, 2, "expected two parcels for a two-warehouse allocation")
	// shipmentsForOrder already orders by created_at, id.
	return storeID, tenantID, orderID, shipments[0].ID, shipments[1].ID
}

// seedShuffledParcels seeds n shipment rows directly (bypassing Create(),
// which has no reason to control id/created_at relationships) for one
// order, choosing ids and created_at values that deliberately DISAGREE.
//
// The first attempt at this fixture only inverted PHYSICAL INSERT order
// against created_at, on the theory that an unordered query falls back to
// heap/physical scan order. Re-running the mutation (drop the explicit
// .Order(...) in GetShipmentByOrderID) against that fixture still only
// caught it ~50-60% of the time — because that theory was wrong. GORM's
// First() silently injects `ORDER BY id ASC` whenever no explicit Order()
// is given (a documented GORM convention for First/Last), so the "no
// ordering" mutant is not actually unordered — it orders by the random
// v4 UUID primary key, which is unrelated to created_at and only
// disagrees with the intended row part of the time.
//
// The reliable fixture therefore has to invert id order against created_at
// order, not insertion order: the row that should sort FIRST by created_at
// is given the LARGEST id, and the row that should sort LAST is given the
// SMALLEST id. That makes the two possible answers provably different
// instead of probabilistically different:
//   - correct code (ORDER BY created_at ASC, id ASC) returns the
//     earliest-created_at row, regardless of its id.
//   - the mutant (GORM's implicit ORDER BY id ASC) returns the
//     smallest-id row, which by construction is the row that should sort
//     LAST — never the same row as the correct answer.
//
// Returns the shipment ids in INTENDED (created_at ASC) order — index 0 is
// the row every correct caller must treat as "the first parcel".
func seedShuffledParcels(t *testing.T, db *gorm.DB, n int) (storeID, tenantID, orderID uuid.UUID, orderedIDs []string) {
	t.Helper()
	storeID, tenantID = seedPerWarehouseStore(t, db)
	orderID = seedPerWarehouseOrder(t, db, storeID, tenantID)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	orderedIDs = make([]string, n)
	for i := 0; i < n; i++ {
		// Intended position i (0 = first/earliest) gets an ASCENDING
		// created_at but a DESCENDING id — id value (n-1-i), so index 0
		// (earliest created_at) holds the LARGEST id and index n-1
		// (latest created_at) holds the SMALLEST id (0000...0000).
		createdAt := base.Add(time.Duration(i) * time.Hour)
		id := deterministicUUID(n - 1 - i)
		rec := shipping.ShipmentRecord{
			ID:             id,
			TenantID:       tenantID,
			StoreID:        storeID,
			OrderID:        orderID,
			Carrier:        "stubcarrier",
			TrackingNumber: fmt.Sprintf("TRK-SHUFFLED-%d", i),
			Status:         "pending",
			ShipFrom:       datatypes.JSON(`{}`),
			ShipTo:         datatypes.JSON(`{}`),
			HandlingFee:    decimal.Zero,
			CurrencyCode:   "INR",
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
		}
		require.NoError(t, db.Create(&rec).Error)
		orderedIDs[i] = rec.ID.String()
	}
	return storeID, tenantID, orderID, orderedIDs
}

// deterministicUUID builds a valid, lexicographically-ordered uuid.UUID
// from a small non-negative integer — value 0 sorts smallest, larger
// values sort larger, ascending in step with the integer. Used so a
// fixture can pin id ordering independently of created_at ordering
// (see seedShuffledParcels).
func deterministicUUID(value int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", value))
}

// getByOrderViaHandler calls GetByOrder directly against a hand-built
// gin.Context, mirroring createShipmentViaHandler's pattern.
func getByOrderViaHandler(t *testing.T, h *ShipmentsHandler, storeID, orderID uuid.UUID, all bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "storeId", Value: storeID.String()},
		{Key: "id", Value: orderID.String()},
	}
	target := "/"
	if all {
		target = "/?all=true"
	}
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)

	h.GetByOrder(c)
	return w
}

// updateStatusViaHandler calls UpdateStatus directly against a hand-built
// gin.Context for the given shipmentId/orderId/storeId path params.
func updateStatusViaHandler(t *testing.T, h *ShipmentsHandler, storeID, orderID uuid.UUID, shipmentID, status string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body, err := json.Marshal(UpdateStatusRequest{Status: status})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "storeId", Value: storeID.String()},
		{Key: "id", Value: orderID.String()},
		{Key: "shipmentId", Value: shipmentID},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateStatus(c)
	return w
}

// TestGetByOrder_AllTrueReturnsBothParcelsOldestFirst covers defect 3: the
// admin previously had no way to fetch every parcel. ?all=true must return
// both, in created_at ASC, id ASC order.
func TestGetByOrder_AllTrueReturnsBothParcelsOldestFirst(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, _, orderID, firstID, secondID := seedTwoParcelOrder(t, db)

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := getByOrderViaHandler(t, h, storeID, orderID, true)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got []ShipmentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 2, "?all=true must return both parcels")
	require.Equal(t, firstID, got[0].ID, "first parcel must be oldest")
	require.Equal(t, secondID, got[1].ID, "second parcel must be second")
}

// TestGetByOrder_WithoutAllReturnsFirstParcelDeterministically covers
// defect 1: GetShipmentByOrderID had no ORDER BY, so it returned an
// arbitrary parcel. Without ?all=true, GetByOrder must consistently return
// the FIRST parcel (by created_at, id) across repeated calls.
//
// Uses seedShuffledParcels (4 rows) rather than seedTwoParcelOrder: id
// order there is the exact REVERSE of intended created_at order, so
// GORM's implicit "ORDER BY id ASC" fallback (what actually runs when the
// explicit .Order(...) is removed — see seedShuffledParcels' doc comment)
// disagrees with the correct answer by construction, not by chance. See
// seedShuffledParcels' doc comment and REPORT.md's Mutation A note for why
// a naively-seeded fixture only caught the regression ~50-60% of the time.
func TestGetByOrder_WithoutAllReturnsFirstParcelDeterministically(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, _, orderID, orderedIDs := seedShuffledParcels(t, db, 4)
	wantID := orderedIDs[0]

	// stubCarrier-independent: seedShuffledParcels stamps each row's own
	// tracking number ("TRK-SHUFFLED-<intended position>"), so asserting on
	// it (not just the id) proves GetByOrder returned the actual first
	// parcel's data, not merely a row with a matching id.
	wantTrackingNumber := trackingNumberForShipment(t, db, wantID)

	h := newPerWarehouseHandler(db, &stubCarrier{})

	for i := 0; i < 5; i++ {
		w := getByOrderViaHandler(t, h, storeID, orderID, false)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var got ShipmentResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Equal(t, wantID, got.ID,
			"call %d: GetByOrder without ?all=true must always return the first parcel", i)
		require.Equal(t, wantTrackingNumber, got.TrackingNumber,
			"call %d: must return the first parcel's own tracking number", i)
		for _, otherID := range orderedIDs[1:] {
			require.NotEqual(t, otherID, got.ID)
		}
	}
}

// TestUpdateStatus_SucceedsForSecondParcel is the core regression for
// defect 2: UpdateStatus resolved the record via GetShipmentByOrderID (by
// ORDER id) and then rejected any :shipmentId that didn't match, so the
// second parcel could never be advanced. It must now resolve by the
// shipment's own id, and must NOT touch the first parcel's status.
func TestUpdateStatus_SucceedsForSecondParcel(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, _, orderID, firstID, secondID := seedTwoParcelOrder(t, db)

	h := newPerWarehouseHandler(db, &stubCarrier{})
	w := updateStatusViaHandler(t, h, storeID, orderID, secondID, "in_transit")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got ShipmentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, secondID, got.ID)
	require.Equal(t, "in_transit", got.Status)

	firstStatus := statusForShipment(t, db, firstID)
	secondStatus := statusForShipment(t, db, secondID)
	require.Equal(t, "in_transit", secondStatus, "the second parcel's status must have advanced")
	require.Equal(t, "pending", firstStatus, "the first parcel's status must NOT have changed")
}

// TestUpdateStatus_MismatchedOrderOrStore404s covers the authorisation
// boundary UpdateStatus must keep: a shipment id that does not belong to
// the order/store in the path still 404s.
func TestUpdateStatus_MismatchedOrderOrStore404s(t *testing.T) {
	db := testdb.NewDB(t, perWarehouseTables...)
	storeID, tenantID, orderID, _, secondID := seedTwoParcelOrder(t, db)

	h := newPerWarehouseHandler(db, &stubCarrier{})

	t.Run("wrong order id", func(t *testing.T) {
		otherOrderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
		w := updateStatusViaHandler(t, h, storeID, otherOrderID, secondID, "in_transit")
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})

	t.Run("wrong store id", func(t *testing.T) {
		otherStoreID := uuid.New()
		w := updateStatusViaHandler(t, h, otherStoreID, orderID, secondID, "in_transit")
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})
}

// statusForShipment reads shipments.status directly for a given shipment
// id — used to assert one parcel's status advanced without disturbing the
// other's.
func statusForShipment(t *testing.T, db *gorm.DB, shipmentID string) string {
	t.Helper()
	var status string
	require.NoError(t, db.Raw(
		`SELECT status FROM shipments WHERE id = ?`, shipmentID).Row().Scan(&status))
	return status
}

// trackingNumberForShipment reads shipments.tracking_number directly.
func trackingNumberForShipment(t *testing.T, db *gorm.DB, shipmentID string) string {
	t.Helper()
	var trackingNumber string
	require.NoError(t, db.Raw(
		`SELECT tracking_number FROM shipments WHERE id = ?`, shipmentID).Row().Scan(&trackingNumber))
	return trackingNumber
}
