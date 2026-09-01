package shipmentcancel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

type recordedSet struct {
	ID             uuid.UUID
	Action, Status string
	Reason         string
}

type fakeStore struct {
	byOrder map[uuid.UUID][]shipping.ShipmentRecord
	byID    map[uuid.UUID]shipping.ShipmentRecord
	sets    []recordedSet
	created []shipping.ShipmentRecord
	// released records every shipment whose allocations were un-stamped,
	// so tests can assert the never-shipped boundary.
	released []uuid.UUID
	// releaseErr makes the release fail, to prove a successful cancel is
	// still reported as succeeded.
	releaseErr error
}

func (f *fakeStore) ReleaseAllocationsForShipment(_ context.Context, id uuid.UUID) (int64, error) {
	if f.releaseErr != nil {
		return 0, f.releaseErr
	}
	f.released = append(f.released, id)
	return 2, nil
}

func (f *fakeStore) CreateShipment(_ context.Context, rec *shipping.ShipmentRecord) error {
	f.created = append(f.created, *rec)
	return nil
}

func (f *fakeStore) ListShipmentsByOrderID(_ context.Context, orderID uuid.UUID) ([]shipping.ShipmentRecord, error) {
	return f.byOrder[orderID], nil
}
func (f *fakeStore) GetShipmentByID(_ context.Context, id uuid.UUID) (*shipping.ShipmentRecord, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &r, nil
}
func (f *fakeStore) SetShipmentCancelState(_ context.Context, id uuid.UUID, action, status, reason string) error {
	f.sets = append(f.sets, recordedSet{id, action, status, reason})
	return nil
}

type fakeCarrier struct {
	calls int
	err   error
}

func (f *fakeCarrier) CancelShipment(_ context.Context, _ string) error { f.calls++; return f.err }
func (f *fakeCarrier) GetRates(context.Context, shipping.RateRequest) ([]shipping.Rate, error) {
	return nil, nil
}
func (f *fakeCarrier) CreateShipment(context.Context, shipping.ShipmentRequest) (*shipping.Shipment, error) {
	return nil, nil
}
func (f *fakeCarrier) GetTracking(context.Context, string) (*shipping.Tracking, error) {
	return nil, nil
}
func (f *fakeCarrier) ProviderName() string         { return "delhivery" }
func (f *fakeCarrier) SupportedCountries() []string { return []string{"IN"} }

func resolverFor(cr shipping.Carrier) CarrierResolver {
	return func(context.Context, uuid.UUID, string) (shipping.Carrier, error) { return cr, nil }
}

func TestExecutor_CancelForward_Success(t *testing.T) {
	oid, sid, stid := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, StoreID: stid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "succeeded" {
		t.Fatalf("outcomes = %+v, want one succeeded", out)
	}
	if car.calls != 1 {
		t.Errorf("carrier calls = %d, want 1", car.calls)
	}
	if len(store.sets) != 1 || store.sets[0].Status != "succeeded" || store.sets[0].Action != string(ActionCancelForward) {
		t.Errorf("recorded = %+v, want cancel_forward/succeeded", store.sets)
	}
}

func TestExecutor_CancelForward_CarrierFailureRecordedNotFatal(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
	}}
	car := &fakeCarrier{err: errors.New("delhivery: cancel shipment: Incorrect Waybill")}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "failed" {
		t.Fatalf("outcomes = %+v, want one failed", out)
	}
	if store.sets[0].Status != "failed" || store.sets[0].Reason == "" {
		t.Errorf("recorded = %+v, want failed with reason", store.sets)
	}
	if store.sets[0].Reason != "Incorrect Waybill" {
		t.Errorf("reason = %q, want cleaned 'Incorrect Waybill'", store.sets[0].Reason)
	}
}

func TestExecutor_InTransit_Unsupported(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "in_transit"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "unsupported" {
		t.Fatalf("outcomes = %+v, want unsupported", out)
	}
	if car.calls != 0 {
		t.Errorf("carrier called %d times for in-transit, want 0", car.calls)
	}
	if store.sets[0].Action != string(ActionTriggerRTO) || store.sets[0].Status != "unsupported" {
		t.Errorf("recorded = %+v, want rto/unsupported", store.sets)
	}
}

func TestExecutor_NoShipments_NoOp(t *testing.T) {
	e := NewExecutor(&fakeStore{}, resolverFor(&fakeCarrier{}), nil)
	if out := e.CancelForOrder(context.Background(), uuid.New()); len(out) != 0 {
		t.Fatalf("outcomes = %+v, want none", out)
	}
}

func TestExecutor_AlreadySucceeded_Idempotent(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending", CancelStatus: "succeeded", CancelAction: "cancel_forward"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if car.calls != 0 {
		t.Errorf("carrier called %d times on already-succeeded, want 0", car.calls)
	}
	if len(out) != 1 || out[0].Status != "succeeded" {
		t.Errorf("outcomes = %+v, want succeeded passthrough", out)
	}
	if len(store.sets) != 0 {
		t.Errorf("re-recorded %+v on already-succeeded, want no write", store.sets)
	}
}

func TestExecutor_CancelShipmentByID_NotFound(t *testing.T) {
	e := NewExecutor(&fakeStore{byID: map[uuid.UUID]shipping.ShipmentRecord{}}, resolverFor(&fakeCarrier{}), nil)
	if _, err := e.CancelShipmentByID(context.Background(), uuid.New()); err == nil {
		t.Fatal("CancelShipmentByID on missing shipment returned nil error, want error")
	}
}

func TestExecutor_CancelShipmentByID_Success(t *testing.T) {
	sid, stid := uuid.New(), uuid.New()
	store := &fakeStore{byID: map[uuid.UUID]shipping.ShipmentRecord{
		sid: {ID: sid, StoreID: stid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out, err := e.CancelShipmentByID(context.Background(), sid)
	if err != nil {
		t.Fatalf("CancelShipmentByID err = %v", err)
	}
	if out.Status != "succeeded" || car.calls != 1 {
		t.Errorf("out = %+v calls = %d, want succeeded/1", out, car.calls)
	}
}

func TestExecutor_MissingTrackingNumber_Failed(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "", Status: "pending"}},
	}}
	car := &fakeCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "failed" {
		t.Fatalf("outcomes = %+v, want failed", out)
	}
	if car.calls != 0 {
		t.Errorf("carrier called with no tracking number, want 0")
	}
}

// fakeRTOCarrier implements shipping.Carrier AND shipping.ReturnToOriginer.
type fakeRTOCarrier struct {
	fakeCarrier
	rtoCalls int
	rtoErr   error
}

func (f *fakeRTOCarrier) ReturnToOrigin(_ context.Context, _ string) error {
	f.rtoCalls++
	return f.rtoErr
}

func TestExecutor_InTransit_RTO_Success(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "in_transit"}},
	}}
	car := &fakeRTOCarrier{}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "succeeded" || out[0].Action != ActionTriggerRTO {
		t.Fatalf("outcomes = %+v, want rto/succeeded", out)
	}
	if car.rtoCalls != 1 {
		t.Errorf("ReturnToOrigin calls = %d, want 1", car.rtoCalls)
	}
	if car.calls != 0 {
		t.Errorf("CancelShipment called %d times, want 0 (RTO path)", car.calls)
	}
	if store.sets[0].Action != string(ActionTriggerRTO) || store.sets[0].Status != "succeeded" {
		t.Errorf("recorded = %+v, want rto/succeeded", store.sets)
	}
}

func TestExecutor_InTransit_RTO_CarrierFailure(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "out_for_delivery"}},
	}}
	car := &fakeRTOCarrier{rtoErr: errors.New("delhivery: cancel shipment: Not cancellable in current state")}
	e := NewExecutor(store, resolverFor(car), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "failed" || out[0].Action != ActionTriggerRTO {
		t.Fatalf("outcomes = %+v, want rto/failed", out)
	}
	if store.sets[0].Reason != "Not cancellable in current state" {
		t.Errorf("reason = %q, want cleaned carrier reason", store.sets[0].Reason)
	}
}

type fakeReverseCarrier struct {
	fakeCarrier
	revCalls int
	revReq   shipping.ReverseShipmentRequest
	revErr   error
}

func (f *fakeReverseCarrier) CreateReverseShipment(_ context.Context, in shipping.ReverseShipmentRequest) (*shipping.Shipment, error) {
	f.revCalls++
	f.revReq = in
	if f.revErr != nil {
		return nil, f.revErr
	}
	return &shipping.Shipment{TrackingNumber: "REV-NEW", ProviderShipmentID: "REV-NEW", Carrier: "delhivery"}, nil
}

func deliveredShipment(sid, stid uuid.UUID) shipping.ShipmentRecord {
	return shipping.ShipmentRecord{
		ID: sid, StoreID: stid, Carrier: "delhivery", TrackingNumber: "WBN-DLV", Status: "delivered",
		ShipFrom: []byte(`{"name":"Warehouse A","line1":"1 Store Rd","city":"Bengaluru","region":"KA","postal_code":"560001","country_code":"IN","phone":"9000000000"}`),
		ShipTo:   []byte(`{"name":"Jane Doe","line1":"42 Lane","city":"Mumbai","region":"MH","postal_code":"400001","country_code":"IN","phone":"9111111111"}`),
	}
}

func TestExecutor_Delivered_ReversePickup_Disabled(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, uuid.New())}}}
	car := &fakeReverseCarrier{}
	e := NewExecutor(store, resolverFor(car), nil) // reverse pickup off by default

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "unsupported" || out[0].Action != ActionReversePickup {
		t.Fatalf("out = %+v, want reverse_pickup/unsupported when disabled", out)
	}
	if car.revCalls != 0 || len(store.created) != 0 {
		t.Errorf("reverse created while disabled (calls=%d rows=%d)", car.revCalls, len(store.created))
	}
}

func TestExecutor_Delivered_ReversePickup_Enabled(t *testing.T) {
	oid, sid, stid := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, stid)}}}
	car := &fakeReverseCarrier{}
	e := NewExecutor(store, resolverFor(car), nil).WithReversePickup(true)

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "succeeded" || out[0].Action != ActionReversePickup {
		t.Fatalf("out = %+v, want reverse_pickup/succeeded", out)
	}
	if car.revCalls != 1 {
		t.Fatalf("CreateReverseShipment calls = %d, want 1", car.revCalls)
	}
	if car.revReq.PickupFrom.PostalCode != "400001" || car.revReq.ReturnTo.PostalCode != "560001" {
		t.Errorf("address mapping wrong: pickup=%q return=%q", car.revReq.PickupFrom.PostalCode, car.revReq.ReturnTo.PostalCode)
	}
	if car.revReq.WarehouseName != "Warehouse A" {
		t.Errorf("warehouse name = %q, want Warehouse A", car.revReq.WarehouseName)
	}
	if car.revReq.OriginalTrackingNumber != "WBN-DLV" {
		t.Errorf("original tracking = %q, want WBN-DLV (needed for ShipEngine return-from-label)", car.revReq.OriginalTrackingNumber)
	}
	if len(store.created) != 1 {
		t.Fatalf("reverse-leg rows = %d, want 1", len(store.created))
	}
	rev := store.created[0]
	if rev.TrackingNumber != "REV-NEW" {
		t.Errorf("reverse-leg row wrong: %+v", rev)
	}
	if rev.CancelStatus != "succeeded" || rev.CancelAction != string(ActionReversePickup) {
		t.Errorf("reverse-leg not marked (status=%q action=%q) — re-run would try to cancel it", rev.CancelStatus, rev.CancelAction)
	}
	if store.sets[0].Action != string(ActionReversePickup) || store.sets[0].Status != "succeeded" {
		t.Errorf("forward record = %+v, want reverse_pickup/succeeded", store.sets)
	}
}

func TestExecutor_Delivered_ReversePickup_CarrierFailure(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, uuid.New())}}}
	car := &fakeReverseCarrier{revErr: errors.New("delhivery: create shipment: Return pin not serviceable")}
	e := NewExecutor(store, resolverFor(car), nil).WithReversePickup(true)

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "failed" {
		t.Fatalf("out = %+v, want failed on carrier error", out)
	}
	if len(store.created) != 0 {
		t.Errorf("reverse-leg row created despite carrier failure")
	}
	if store.sets[0].Reason != "Return pin not serviceable" {
		t.Errorf("reason = %q, want cleaned carrier reason", store.sets[0].Reason)
	}
}

func TestExecutor_Delivered_NoReverseCapability_Unsupported(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{oid: {deliveredShipment(sid, uuid.New())}}}
	car := &fakeCarrier{} // no CreateReverseShipment
	e := NewExecutor(store, resolverFor(car), nil).WithReversePickup(true)

	out := e.CancelForOrder(context.Background(), oid)
	if out[0].Status != "unsupported" || out[0].Action != ActionReversePickup {
		t.Fatalf("out = %+v, want reverse_pickup/unsupported for a carrier without the capability", out)
	}
}

// TestParseShipmentAddress_RejectsAnErasedBlob pins the guard added for #435.
//
// A GDPR art.17 erasure strips shipments.ship_to / ship_from to '{}' — the
// columns are NOT NULL, so it cannot use NULL. That is two bytes of perfectly
// valid JSON, and without the line1 check it would decode into a blank
// Address and be handed to a carrier as a real pickup point. The reverse-leg
// path must instead fail and tell the operator to arrange the return by hand.
func TestParseShipmentAddress_RejectsAnErasedBlob(t *testing.T) {
	for _, raw := range []string{"", "{}", `{"country_code":"IE"}`} {
		if _, err := parseShipmentAddress([]byte(raw)); !errors.Is(err, errEmptyAddress) {
			t.Fatalf("parseShipmentAddress(%q) = %v, want errEmptyAddress — a blob with no line1 is not an address", raw, err)
		}
	}

	got, err := parseShipmentAddress([]byte(`{"name":"A Person","line1":"1 Test Lane","city":"Dublin","country_code":"IE"}`))
	if err != nil {
		t.Fatalf("a populated blob must still parse: %v", err)
	}
	if got.Line1 != "1 Test Lane" || got.Name != "A Person" {
		t.Fatalf("populated address decoded wrong: %+v", got)
	}
}

// --- allocation release on cancel (#497) ---
//
// Cancelling used to leave order_allocations stamped with shipment_id, so
// Create() returned 409 already_shipped forever and the only escape was
// deleting the shipment. A cancelled shipment is not a shipped one.

func TestExecutor_CancelForward_ReleasesAllocations(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
	}}
	e := NewExecutor(store, resolverFor(&fakeCarrier{}), nil)

	e.CancelForOrder(context.Background(), oid)

	if len(store.released) != 1 || store.released[0] != sid {
		t.Errorf("released = %v, want [%s]", store.released, sid)
	}
}

// The boundary that matters. in_transit resolves to ActionTriggerRTO and
// delivered to ActionReversePickup — the goods are moving or already
// there. Freeing those allocations would let a merchant create a SECOND
// label for the same goods.
func TestExecutor_ShippedStatuses_DoNotReleaseAllocations(t *testing.T) {
	for _, status := range []string{"in_transit", "out_for_delivery", "delivered"} {
		t.Run(status, func(t *testing.T) {
			oid, sid := uuid.New(), uuid.New()
			store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
				oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: status}},
			}}
			e := NewExecutor(store, resolverFor(&fakeCarrier{}), nil)

			e.CancelForOrder(context.Background(), oid)

			if len(store.released) != 0 {
				t.Errorf("released = %v for status %q, want none", store.released, status)
			}
		})
	}
}

// A carrier that refused the cancel means the label still stands, so the
// allocations must stay stamped.
func TestExecutor_FailedCancel_DoesNotReleaseAllocations(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
		oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
	}}
	car := &fakeCarrier{err: errors.New("delhivery: cancel shipment: Incorrect Waybill")}
	e := NewExecutor(store, resolverFor(car), nil)

	e.CancelForOrder(context.Background(), oid)

	if len(store.released) != 0 {
		t.Errorf("released = %v after a failed cancel, want none", store.released)
	}
}

// The cancel already succeeded at the carrier by this point. A failure to
// free allocations must not downgrade that to an error — the merchant is
// left in the old (recoverable) dead end rather than being told a
// successful cancel failed.
func TestExecutor_ReleaseFailure_StillReportsSucceeded(t *testing.T) {
	oid, sid := uuid.New(), uuid.New()
	store := &fakeStore{
		byOrder: map[uuid.UUID][]shipping.ShipmentRecord{
			oid: {{ID: sid, Carrier: "delhivery", TrackingNumber: "WBN1", Status: "pending"}},
		},
		releaseErr: errors.New("db down"),
	}
	e := NewExecutor(store, resolverFor(&fakeCarrier{}), nil)

	out := e.CancelForOrder(context.Background(), oid)
	if len(out) != 1 || out[0].Status != "succeeded" {
		t.Fatalf("outcomes = %+v, want one succeeded despite release failure", out)
	}
}
