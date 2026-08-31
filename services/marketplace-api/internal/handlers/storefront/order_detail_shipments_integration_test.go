//go:build integration

// Package storefront — coverage for exposing every shipment on an order
// (#177 PR 4a).
//
// Until now the response carried one `shipment`, chosen as the most recent.
// Multi-warehouse orders ship as more than one parcel, and a customer seeing
// only one tracking number has silently lost the other.
//
// `shipment` is deliberately UNCHANGED: seven call sites in apps/storefront
// read it, including invoice rendering, and the API deploys independently of
// the app. The list is added alongside so the app can migrate on its own
// schedule.
package storefront

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedShipmentsOrder creates a store and an order, and returns (storeID, orderID).
func seedShipmentsOrder(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Ship List Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "shiplist-"+uuid.NewString()[:8], uuid.NewString()).Error)

	orderID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 10.00, 10.00)`,
		orderID, tenantID, storeID, "SL-"+uuid.NewString()[:8], uuid.NewString()).Error)
	return storeID, orderID
}

func seedShipment(t *testing.T, db *gorm.DB, storeID, orderID, carrier, tracking string, createdAt time.Time) {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))
	require.NoError(t, db.Exec(
		`INSERT INTO shipments (id, tenant_id, store_id, order_id, carrier, tracking_number,
		                        status, ship_from, ship_to, handling_fee, currency_code, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}'::jsonb, '{}'::jsonb, 0, 'INR', ?)`,
		uuid.NewString(), tenantID, storeID, orderID, carrier, tracking, createdAt).Error)
}

func TestLoadShipments_ReturnsEveryShipmentOldestFirst(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, orderID := seedShipmentsOrder(t, db)

	base := time.Now().Add(-time.Hour)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-FIRST", base)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-SECOND", base.Add(time.Minute))

	h := &OrderDetailHandler{db: db}
	got := h.loadShipments(context.Background(), uuid.MustParse(orderID))

	require.Len(t, got, 2, "a two-parcel order must expose both shipments")
	require.Equal(t, "AWB-FIRST", got[0].TrackingNumber, "oldest first, so parcel order is stable as more are added")
	require.Equal(t, "AWB-SECOND", got[1].TrackingNumber)
}

func TestLoadShipments_EmptyForAnUnshippedOrder(t *testing.T) {
	db := testdb.NewTx(t)
	_, orderID := seedShipmentsOrder(t, db)

	h := &OrderDetailHandler{db: db}
	got := h.loadShipments(context.Background(), uuid.MustParse(orderID))

	require.Empty(t, got, "an unshipped order has no parcels, and must not error")
}

// The singular field is what apps/storefront reads today — including invoice
// rendering. If this diverges, invoices break the moment the API deploys
// ahead of the app.
func TestLoadShipment_StillReturnsTheMostRecent(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, orderID := seedShipmentsOrder(t, db)

	base := time.Now().Add(-time.Hour)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-FIRST", base)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-SECOND", base.Add(time.Minute))

	h := &OrderDetailHandler{db: db}
	got := h.loadShipment(context.Background(), uuid.MustParse(orderID))

	require.NotNil(t, got)
	require.Equal(t, "AWB-SECOND", got.TrackingNumber,
		"the singular field is unchanged: most recent, exactly as before this PR")
}

// The list and the singular field must agree about the same parcel, or a
// customer sees one status in the summary and another in the detail.
func TestLoadShipments_LastEntryMatchesTheSingularField(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, orderID := seedShipmentsOrder(t, db)

	base := time.Now().Add(-time.Hour)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-FIRST", base)
	seedShipment(t, db, storeID, orderID, "delhivery", "AWB-SECOND", base.Add(time.Minute))

	h := &OrderDetailHandler{db: db}
	list := h.loadShipments(context.Background(), uuid.MustParse(orderID))
	single := h.loadShipment(context.Background(), uuid.MustParse(orderID))

	require.NotEmpty(t, list)
	require.NotNil(t, single)
	require.Equal(t, list[len(list)-1].TrackingNumber, single.TrackingNumber)
}

// The no-omitempty tag is only half the contract: a nil slice still
// marshals to null, and a storefront calling .map() on null throws. An
// unshipped order must serialise an empty ARRAY.
func TestOrderDetailResponse_EmptyShipmentsMarshalsAsArrayNotNull(t *testing.T) {
	db := testdb.NewTx(t)
	_, orderID := seedShipmentsOrder(t, db)

	h := &OrderDetailHandler{db: db}
	resp := storefrontOrderResponse{
		Shipments: h.loadShipments(context.Background(), uuid.MustParse(orderID)),
	}

	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(body), `"shipments":[]`,
		"an unshipped order must serialise an empty array, not null")
}
