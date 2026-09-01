package public

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// fakeShippingRepo satisfies the slice of the Repository surface the
// webhook actually uses. Anything else panics so a future handler
// refactor that starts relying on additional methods trips this
// test before prod.
type fakeShippingRepo struct {
	shipments map[string]*shipping.ShipmentRecord
	configs   map[string]*shipping.CarrierConfig
}

func newFakeRepo() *fakeShippingRepo {
	return &fakeShippingRepo{
		shipments: map[string]*shipping.ShipmentRecord{},
		configs:   map[string]*shipping.CarrierConfig{},
	}
}

func (f *fakeShippingRepo) GetShipmentByTrackingNumber(_ context.Context, carrier, tn string) (*shipping.ShipmentRecord, error) {
	rec, ok := f.shipments[carrier+"/"+tn]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return rec, nil
}

func (f *fakeShippingRepo) GetCarrierConfig(_ context.Context, storeID, provider string) (*shipping.CarrierConfig, error) {
	cfg, ok := f.configs[storeID+"/"+provider]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return cfg, nil
}

// Unused methods — present to satisfy the interface. Any call here
// indicates the webhook handler grew a dependency we're no longer
// modelling in the test, so panic loud.
func (f *fakeShippingRepo) CreateShipment(context.Context, *shipping.ShipmentRecord) error {
	panic("not implemented")
}
func (f *fakeShippingRepo) GetShipmentByID(context.Context, uuid.UUID) (*shipping.ShipmentRecord, error) {
	panic("not implemented")
}
func (f *fakeShippingRepo) GetShipmentByOrderID(context.Context, uuid.UUID) (*shipping.ShipmentRecord, error) {
	panic("not implemented")
}
func (f *fakeShippingRepo) ListShipmentsByOrderID(context.Context, uuid.UUID) ([]shipping.ShipmentRecord, error) {
	panic("not implemented")
}
func (f *fakeShippingRepo) ListShipmentsByStore(context.Context, uuid.UUID, int, int) ([]shipping.ShipmentRecord, int64, error) {
	panic("not implemented")
}
func (f *fakeShippingRepo) UpdateShipmentStatus(context.Context, uuid.UUID, string) error {
	panic("not implemented")
}
func (f *fakeShippingRepo) SetShipmentCancelState(context.Context, uuid.UUID, string, string, string) error {
	panic("not implemented")
}

func (f *fakeShippingRepo) ReleaseAllocationsForShipment(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}
func (f *fakeShippingRepo) ListCarrierConfigs(context.Context, string) ([]shipping.CarrierConfig, error) {
	panic("not implemented")
}
func (f *fakeShippingRepo) UpsertCarrierConfig(context.Context, *shipping.CarrierConfig) error {
	panic("not implemented")
}
func (f *fakeShippingRepo) DeleteCarrierConfig(context.Context, uuid.UUID) error {
	panic("not implemented")
}

// makeWebhookHandler builds a test handler with an injected advance
// spy. Spy records every advance call so tests can assert precisely
// which transitions fired.
type advanceRecord struct {
	rec      shipping.ShipmentRecord
	tracking shipping.Tracking
}

func makeWebhookHandler(t *testing.T, secret string) (*DelhiveryWebhookHandler, *fakeShippingRepo, *[]advanceRecord, *atomic.Int32) {
	t.Helper()
	repo := newFakeRepo()
	enc := crypto.NewNoopEncryptor()
	// Encrypt the secret so the handler's resolveSecret path is
	// exercised end-to-end — mirroring the real carrier config
	// shape where secret_key is always stored as ciphertext.
	cipher, err := enc.Encrypt(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	storeID := uuid.New()
	repo.configs[storeID.String()+"/delhivery"] = &shipping.CarrierConfig{
		ID: uuid.New(), StoreID: storeID, Provider: "delhivery",
		APIKey: "x", SecretKey: cipher, Mode: "test",
	}

	tenantID := uuid.New()
	repo.shipments["delhivery/AWB-OK"] = &shipping.ShipmentRecord{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID, OrderID: uuid.New(),
		Carrier: "delhivery", TrackingNumber: "AWB-OK", Status: "in_transit",
	}

	var calls atomic.Int32
	var records []advanceRecord
	advance := func(_ context.Context, rec *shipping.ShipmentRecord, tr *shipping.Tracking) (bool, error) {
		records = append(records, advanceRecord{rec: *rec, tracking: *tr})
		calls.Add(1)
		return tr.Status != rec.Status, nil
	}

	h := NewDelhiveryWebhookHandler(nil, repo, nil).
		WithEncryptor(enc).
		WithAdvance(advance)
	return h, repo, &records, &calls
}

// TestDelhiveryWebhook_DeliveredAdvancesShipment pins the happy path:
// a valid token + known AWB fires the advance callback once with the
// mapped status so downstream wiring (order_events, receipt email)
// runs exactly as if a human had clicked "Refresh tracking".
func TestDelhiveryWebhook_DeliveredAdvancesShipment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, records, calls := makeWebhookHandler(t, "webhook-secret-123")

	r := gin.New()
	r.POST("/api/v1/carrier-webhooks/delhivery", h.Handle)

	body := []byte(`{
		"Shipment": {
			"AWB": "AWB-OK",
			"Status": {"Status": "Delivered", "StatusType": "DL", "Instructions": "Delivered to consignee"}
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/carrier-webhooks/delhivery", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Token", "webhook-secret-123")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// Advance fires in a detached goroutine for the production path;
	// poll briefly so the test doesn't race the background dispatch.
	// 200 × 5ms = 1s upper bound — more than enough for a local go
	// scheduler to run the closure.
	for i := 0; i < 200 && calls.Load() == 0; i++ {
		sleepShort()
	}
	if calls.Load() != 1 {
		t.Fatalf("advance calls = %d, want 1", calls.Load())
	}
	if got := (*records)[0].tracking.Status; got != "delivered" {
		t.Fatalf("mapped status = %q, want delivered", got)
	}
}

// TestDelhiveryWebhook_InvalidSecretRejected guarantees we never
// touch the carrier config lookup result if the token doesn't
// validate — an unauthenticated POST should look identical whether
// the AWB exists or not, from the caller's perspective.
func TestDelhiveryWebhook_InvalidSecretRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, _, calls := makeWebhookHandler(t, "real-secret")

	r := gin.New()
	r.POST("/api/v1/carrier-webhooks/delhivery", h.Handle)

	body := []byte(`{"Shipment":{"AWB":"AWB-OK","Status":{"StatusType":"DL"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/carrier-webhooks/delhivery", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Token", "wrong-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("advance fired despite auth failure (calls=%d)", calls.Load())
	}
}

// TestDelhiveryWebhook_UnknownAWBReturns200 pins the anti-enumeration
// behaviour: an AWB we don't know about must LOOK identical to a
// valid one in both status and timing, so attackers can't probe the
// DB via webhook POSTs.
func TestDelhiveryWebhook_UnknownAWBReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, _, calls := makeWebhookHandler(t, "sec")

	r := gin.New()
	r.POST("/api/v1/carrier-webhooks/delhivery", h.Handle)

	body := []byte(`{"Shipment":{"AWB":"DOES-NOT-EXIST","Status":{"StatusType":"DL"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/carrier-webhooks/delhivery", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Token", "sec")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("advance fired for unknown AWB (calls=%d)", calls.Load())
	}
	// Body must look identical to the happy path so timing-side
	// channels don't leak existence either.
	var env map[string]bool
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if !env["accepted"] {
		t.Fatalf("response body = %s, want {\"accepted\":true}", w.Body.String())
	}
}

// TestDelhiveryWebhook_TokenFromQueryString accepts the
// ?token=<secret> form some merchants use in one.delhivery.com →
// Post Webhook when their panel doesn't support custom headers.
// Without this codepath, every such merchant's webhooks would 401.
func TestDelhiveryWebhook_TokenFromQueryString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, _, calls := makeWebhookHandler(t, "qs-secret")

	r := gin.New()
	r.POST("/api/v1/carrier-webhooks/delhivery", h.Handle)

	body := []byte(`{"Shipment":{"AWB":"AWB-OK","Status":{"StatusType":"UT"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/carrier-webhooks/delhivery?token=qs-secret", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	for i := 0; i < 200 && calls.Load() == 0; i++ {
		sleepShort()
	}
	if calls.Load() != 1 {
		t.Fatalf("advance calls = %d, want 1", calls.Load())
	}
}

// sleepShort hands control back to the scheduler so the background
// advance goroutine gets a turn. Deliberately tiny so the polling
// loop converges quickly on happy CI and doesn't pad total test
// time with idle sleeps.
func sleepShort() {
	time.Sleep(5 * time.Millisecond)
}
