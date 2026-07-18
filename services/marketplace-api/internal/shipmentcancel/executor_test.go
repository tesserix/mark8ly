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
func (f *fakeCarrier) ProviderName() string        { return "delhivery" }
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
