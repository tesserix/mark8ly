//go:build integration

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/delivery"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// countingMailer counts shipped emails; the rest of the contract is unused.
type countingMailer struct{ shipped atomic.Int32 }

func (m *countingMailer) SendShipmentDispatched(context.Context, orderdoc.DocumentInput) error {
	m.shipped.Add(1)
	return nil
}
func (m *countingMailer) SendInvoice(context.Context, orderdoc.DocumentInput) error      { return nil }
func (m *countingMailer) SendReceipt(context.Context, orderdoc.DocumentInput) error      { return nil }
func (m *countingMailer) SendCancellation(context.Context, orderdoc.DocumentInput) error { return nil }
func (m *countingMailer) SendRefund(context.Context, orderdoc.DocumentInput) error       { return nil }

// emailsAfter waits out the fire-and-forget dispatch goroutine, then reads the count.
func (m *countingMailer) emailsAfter() int {
	time.Sleep(500 * time.Millisecond)
	return int(m.shipped.Load())
}

var manualShipmentTables = append([]string{"store_couriers"}, perWarehouseTables...)

type manualEnv struct {
	db                *gorm.DB
	h                 *ShipmentsHandler
	mailer            *countingMailer
	storeID, tenantID uuid.UUID
	orderID           uuid.UUID
}

func setupManualEnv(t *testing.T, carrier string) *manualEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, manualShipmentTables...)
	storeID, tenantID := seedPerWarehouseStore(t, db)
	wh := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Main", "1 Main Road")
	wh2 := seedPerWarehouseWarehouse(t, db, storeID, tenantID, "Second", "2 Second Road")
	orderID := seedPerWarehouseOrder(t, db, storeID, tenantID)
	require.NoError(t, db.Exec(`UPDATE orders SET shipping_carrier = ? WHERE id = ?`, carrier, orderID).Error)
	i1 := seedPerWarehouseItem(t, db, orderID, "SKU-1", 1)
	i2 := seedPerWarehouseItem(t, db, orderID, "SKU-2", 2)
	seedAllocation(t, db, tenantID, storeID, orderID, i1, wh.ID, 1)
	seedAllocation(t, db, tenantID, storeID, orderID, i2, wh2.ID, 2)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mailer := &countingMailer{}
	brandingSvc := branding.NewService(branding.ServiceConfig{DB: db, Repo: branding.NewRepository(), Logger: logger})
	docSvc := orderdoc.NewService(db, mailer, order.NewRepository(), brandingSvc, "https://example.test")

	h := newPerWarehouseHandler(db, &stubCarrier{})
	h.docMailer = docSvc
	h.WithDelivery(delivery.NewService(db))
	return &manualEnv{db: db, h: h, mailer: mailer, storeID: storeID, tenantID: tenantID, orderID: orderID}
}

func (e *manualEnv) courier(t *testing.T, storeID uuid.UUID, name string, template *string) string {
	t.Helper()
	c, err := delivery.NewService(e.db).CreateCourier(context.Background(), e.tenantID.String(), storeID.String(),
		delivery.CourierInput{Name: name, TrackingURLTemplate: template})
	require.NoError(t, err)
	return c.ID
}

func (e *manualEnv) call(t *testing.T, method string, params gin.Params, body any, fn func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = append(gin.Params{{Key: "storeId", Value: e.storeID.String()}, {Key: "id", Value: e.orderID.String()}}, params...)
	c.Set("tenant_id", e.tenantID.String())
	c.Request = httptest.NewRequest(method, "/", bytes.NewReader(b))
	fn(c)
	return w
}

func (e *manualEnv) ship(t *testing.T, body any) *httptest.ResponseRecorder {
	return e.call(t, http.MethodPost, nil, body, e.h.CreateManual)
}

func (e *manualEnv) patchTracking(t *testing.T, shipmentID string, body any) *httptest.ResponseRecorder {
	return e.call(t, http.MethodPatch, gin.Params{{Key: "shipmentId", Value: shipmentID}}, body, e.h.UpdateTracking)
}

type ownShipmentRow struct {
	ID             string
	Carrier        string
	CourierName    *string
	TrackingNumber string
	TrackingURL    *string
	Status         string
	ShippedAt      *time.Time
	ShipTo         string
}

func (e *manualEnv) shipments(t *testing.T) []ownShipmentRow {
	t.Helper()
	var rows []ownShipmentRow
	require.NoError(t, e.db.Raw(`SELECT id, carrier, courier_name, tracking_number, tracking_url, status, shipped_at,
		ship_to::text AS ship_to FROM shipments WHERE order_id = ?`, e.orderID).Scan(&rows).Error)
	return rows
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Error string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Error
}

func tmpl(s string) *string { return &s }

func TestManualShipment_CreatesShippedAndEmailsOnce(t *testing.T) {
	e := setupManualEnv(t, "own")
	cid := e.courier(t, e.storeID, "Anjani", tmpl("https://anjani.example/track?awb={number}"))

	w := e.ship(t, gin.H{"courier_id": cid, "tracking_number": " AWB 123 "})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp ShipmentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "Anjani", resp.CourierName)
	require.Equal(t, "https://anjani.example/track?awb=AWB+123", resp.TrackingURL)

	rows := e.shipments(t)
	require.Len(t, rows, 1)
	r := rows[0]
	require.Equal(t, "own", r.Carrier)
	require.Equal(t, "Anjani", *r.CourierName)
	require.Equal(t, "AWB 123", r.TrackingNumber)
	require.NotNil(t, r.TrackingURL)
	require.Equal(t, "https://anjani.example/track?awb=AWB+123", *r.TrackingURL)
	require.Equal(t, "in_transit", r.Status)
	require.NotNil(t, r.ShippedAt)
	require.Contains(t, r.ShipTo, "42 Example Lane")

	var unstamped int64
	require.NoError(t, e.db.Table("order_allocations").
		Where("order_id = ? AND (shipment_id IS NULL OR shipment_id <> ?)", e.orderID, r.ID).Count(&unstamped).Error)
	require.Zero(t, unstamped, "every allocation must link to the manual shipment")
	require.Equal(t, "fulfilled", fulfillmentStatusForOrder(t, e.db, e.orderID))
	require.Equal(t, "confirmed", orderStatusForOrder(t, e.db, e.orderID))
	require.Equal(t, 1, e.mailer.emailsAfter())
}

func TestManualShipment_RefusedForCarrierOrder(t *testing.T) {
	e := setupManualEnv(t, "delhivery")
	cid := e.courier(t, e.storeID, "Anjani", nil)
	w := e.ship(t, gin.H{"courier_id": cid, "tracking_number": "X1"})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Equal(t, "not_own_delivery", errCode(t, w))
	require.Empty(t, e.shipments(t))
}

func TestManualShipment_RefusedWhenActiveShipmentExists(t *testing.T) {
	e := setupManualEnv(t, "own")
	cid := e.courier(t, e.storeID, "Anjani", nil)
	require.Equal(t, http.StatusCreated, e.ship(t, gin.H{"courier_id": cid, "tracking_number": "A1"}).Code)

	w := e.ship(t, gin.H{"courier_id": cid, "tracking_number": "A2"})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Equal(t, "already_shipped", errCode(t, w))
	require.Len(t, e.shipments(t), 1)
	require.Equal(t, 1, e.mailer.emailsAfter())
}

func TestManualShipment_ConcurrentPostsShipOnce(t *testing.T) {
	e := setupManualEnv(t, "own")
	cid := e.courier(t, e.storeID, "Anjani", nil)

	codes := make([]int, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = e.ship(t, gin.H{"courier_id": cid, "tracking_number": "C1"}).Code
		}(i)
	}
	close(start)
	wg.Wait()

	require.ElementsMatch(t, []int{http.StatusCreated, http.StatusConflict}, codes)
	require.Len(t, e.shipments(t), 1)
	require.Equal(t, 1, e.mailer.emailsAfter())
}

func TestManualShipment_CourierFromAnotherStore(t *testing.T) {
	e := setupManualEnv(t, "own")
	otherStore, _ := seedPerWarehouseStore(t, e.db)
	cid := e.courier(t, otherStore, "Elsewhere", nil)
	w := e.ship(t, gin.H{"courier_id": cid, "tracking_number": "X1"})
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Empty(t, e.shipments(t))
}

func TestManualShipment_NoTemplateStoresNumberOnly(t *testing.T) {
	e := setupManualEnv(t, "own")
	cid := e.courier(t, e.storeID, "Bike guy", nil)
	w := e.ship(t, gin.H{"courier_id": cid, "tracking_number": "N1"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	rows := e.shipments(t)
	require.Len(t, rows, 1)
	require.Equal(t, "N1", rows[0].TrackingNumber)
	require.Nil(t, rows[0].TrackingURL)
}

func TestManualShipment_DeleteThenShipAgain(t *testing.T) {
	e := setupManualEnv(t, "own")
	cid := e.courier(t, e.storeID, "Anjani", nil)
	w := e.ship(t, gin.H{"courier_id": cid, "tracking_number": "D1"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := e.shipments(t)[0].ID

	del := e.call(t, http.MethodDelete, gin.Params{{Key: "shipmentId", Value: id}}, nil, e.h.Delete)
	require.Equal(t, http.StatusOK, del.Code, del.Body.String())

	var linked int64
	require.NoError(t, e.db.Table("order_allocations").
		Where("order_id = ? AND shipment_id IS NOT NULL", e.orderID).Count(&linked).Error)
	require.Zero(t, linked, "deleting the own shipment must unlink its allocations")

	w = e.ship(t, gin.H{"courier_id": cid, "tracking_number": "D2"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	rows := e.shipments(t)
	require.Len(t, rows, 1)
	require.Equal(t, "D2", rows[0].TrackingNumber)
}

func TestTrackingPatch_FixesNumberWithoutEmail(t *testing.T) {
	e := setupManualEnv(t, "own")
	cid := e.courier(t, e.storeID, "Anjani", tmpl("https://anjani.example/t/{number}"))
	other := e.courier(t, e.storeID, "Blue Dart", tmpl("https://bluedart.example/{number}"))
	require.Equal(t, http.StatusCreated, e.ship(t, gin.H{"courier_id": cid, "tracking_number": "TYPO"}).Code)
	id := e.shipments(t)[0].ID

	w := e.patchTracking(t, id, gin.H{"tracking_number": "FIXED"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	r := e.shipments(t)[0]
	require.Equal(t, "FIXED", r.TrackingNumber)
	require.Equal(t, "https://anjani.example/t/FIXED", *r.TrackingURL)

	w = e.patchTracking(t, id, gin.H{"courier_id": other, "tracking_number": "BD9"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	r = e.shipments(t)[0]
	require.Equal(t, "Blue Dart", *r.CourierName)
	require.Equal(t, "https://bluedart.example/BD9", *r.TrackingURL)
	require.Equal(t, 1, e.mailer.emailsAfter(), "a tracking fix must not re-send the shipped email")

	// A carrier shipment cannot be edited here.
	carrierRec := &shipping.ShipmentRecord{
		TenantID: e.tenantID, StoreID: e.storeID, OrderID: e.orderID, Carrier: "delhivery",
		TrackingNumber: "DLV1", Status: "pending", CurrencyCode: "INR",
		ShipFrom: []byte(`{}`), ShipTo: []byte(`{}`),
	}
	require.NoError(t, e.db.Create(carrierRec).Error)
	w = e.patchTracking(t, carrierRec.ID.String(), gin.H{"tracking_number": "NOPE"})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Equal(t, "not_own_delivery", errCode(t, w))
}
