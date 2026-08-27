//go:build integration

package admin_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestShipmentDispatchedEmailGate pins the dedup behavior of the
// shipments.dispatched_email_sent_at column.
//
// Two paths can transition a shipment to in_transit:
//  1. admin manually marks shipped via PATCH .../shipments/:id/status
//  2. unified AdvanceShipmentFromTracking helper invoked by the
//     2-min carrier sync loop AND the public Delhivery webhook handler
//
// Without a dedup column, both paths firing in quick succession would
// email the customer twice. dispatchShipmentDispatchedEmail gates on a
// single atomic UPDATE … WHERE dispatched_email_sent_at IS NULL —
// the first transition to win the row sends the email; the second
// caller sees 0 rows affected and silently skips.
//
// This test pins the SQL gate behavior at the DB layer. If a future
// change drops the WHERE clause or adds a non-atomic step, the test
// catches it: a second claim against the already-stamped row should
// affect 0 rows.
func TestShipmentDispatchedEmailGate(t *testing.T) {
	db := testdb.NewDB(t, "shipments")

	tenantID := uuid.New()
	storeID := uuid.New()
	orderID := uuid.New()
	shipmentID := uuid.New()

	ship := shipping.ShipmentRecord{
		ID:             shipmentID,
		TenantID:       tenantID,
		StoreID:        storeID,
		OrderID:        orderID,
		Carrier:        "delhivery",
		TrackingNumber: "DEDUP-AWB-1",
		Status:         "pending",
		ShipFrom:       datatypes.JSON([]byte(`{}`)),
		ShipTo:         datatypes.JSON([]byte(`{}`)),
		CurrencyCode:   "INR",
	}
	seedOrderRowForSync(t, db, orderID, storeID, tenantID)
	if err := db.Create(&ship).Error; err != nil {
		t.Fatalf("seed shipment: %v", err)
	}

	// ─── First claim — should win ──────────────────────────────────────
	now1 := time.Now().UTC()
	res1 := db.Table("shipments").
		Where("id = ? AND dispatched_email_sent_at IS NULL", shipmentID).
		Update("dispatched_email_sent_at", now1)
	if err := res1.Error; err != nil {
		t.Fatalf("first claim update: %v", err)
	}
	if res1.RowsAffected != 1 {
		t.Fatalf("first claim: RowsAffected = %d, want 1 — gate did not stamp on a fresh row", res1.RowsAffected)
	}

	// Verify the timestamp landed.
	var stampedAt *time.Time
	if err := db.Table("shipments").
		Select("dispatched_email_sent_at").
		Where("id = ?", shipmentID).
		Row().Scan(&stampedAt); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if stampedAt == nil {
		t.Fatalf("dispatched_email_sent_at is NULL after first claim; expected stamp")
	}
	firstStamp := *stampedAt

	// ─── Second claim — should be a no-op ──────────────────────────────
	// Sleep a beat so a second UPDATE that ignored the WHERE clause
	// would visibly bump the timestamp. Without this, a regression that
	// drops the IS NULL guard could happen to write the same timestamp
	// twice and pass.
	time.Sleep(20 * time.Millisecond)

	now2 := time.Now().UTC()
	if !now2.After(firstStamp) {
		t.Fatalf("test setup error: now2 (%v) is not after firstStamp (%v)", now2, firstStamp)
	}

	res2 := db.Table("shipments").
		Where("id = ? AND dispatched_email_sent_at IS NULL", shipmentID).
		Update("dispatched_email_sent_at", now2)
	if err := res2.Error; err != nil {
		t.Fatalf("second claim update: %v", err)
	}
	if res2.RowsAffected != 0 {
		t.Fatalf("second claim: RowsAffected = %d, want 0 — gate did not block a re-claim", res2.RowsAffected)
	}

	// Verify the timestamp didn't move.
	var stillAt *time.Time
	if err := db.Table("shipments").
		Select("dispatched_email_sent_at").
		Where("id = ?", shipmentID).
		Row().Scan(&stillAt); err != nil {
		t.Fatalf("read stamp after second claim: %v", err)
	}
	if stillAt == nil {
		t.Fatalf("dispatched_email_sent_at is NULL after second claim — first stamp was lost")
	}
	if !stillAt.Equal(firstStamp) {
		t.Fatalf("dispatched_email_sent_at moved on second claim: was %v, is %v", firstStamp, *stillAt)
	}
}
