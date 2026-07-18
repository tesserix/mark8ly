package shipmentcancel

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// errEmptyAddress is returned by parseShipmentAddress when the stored ship_from
// / ship_to JSONB blob is empty — a shipment we can't build a reverse leg from.
var errEmptyAddress = errors.New("empty address")

// ShipmentStore is the narrow persistence surface the executor needs.
// shipping.Repository satisfies it.
type ShipmentStore interface {
	ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]shipping.ShipmentRecord, error)
	GetShipmentByID(ctx context.Context, id uuid.UUID) (*shipping.ShipmentRecord, error)
	SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error
	CreateShipment(ctx context.Context, rec *shipping.ShipmentRecord) error
}

// CarrierResolver builds a carrier client for a (store, provider). Kept as a
// func so the executor stays free of credential/secret-store dependencies and
// is trivially fakeable in tests.
type CarrierResolver func(ctx context.Context, storeID uuid.UUID, provider string) (shipping.Carrier, error)

// Cancel outcome statuses (also the shipments.cancel_status values).
const (
	statusNone        = "none"
	statusSucceeded   = "succeeded"
	statusFailed      = "failed"
	statusUnsupported = "unsupported"
)

// Outcome is the per-shipment result the manual endpoint aggregates into its
// response. The refund/cancel hooks ignore it (fire-and-record).
type Outcome struct {
	ShipmentID uuid.UUID `json:"shipment_id"`
	Action     Action    `json:"action"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
}

// Executor resolves and executes the carrier action for shipments. Every path
// is best-effort: it records the outcome and never returns a fatal error to
// the refund/cancel caller.
type Executor struct {
	store                ShipmentStore
	resolve              CarrierResolver
	logger               *slog.Logger
	reversePickupEnabled bool
}

// NewExecutor constructs an Executor. logger may be nil.
func NewExecutor(store ShipmentStore, resolve CarrierResolver, logger *slog.Logger) *Executor {
	return &Executor{store: store, resolve: resolve, logger: logger}
}

// WithReversePickup enables the Phase 3 delivered-shipment reverse pickup. OFF
// by default: creating a reverse pickup dispatches a real courier to the
// customer and the Delhivery payload is not yet live-verified. Mirrors the
// REFUND_GATEWAY_ENABLED kill switch. When off, delivered shipments record
// `unsupported`.
func (e *Executor) WithReversePickup(enabled bool) *Executor {
	e.reversePickupEnabled = enabled
	return e
}

// CancelForOrder resolves + executes the action for every shipment on the
// order, independently. Best-effort: a list error or a per-shipment failure is
// logged and recorded, never propagated. Returns one Outcome per shipment
// (empty slice when the order has no shipments).
func (e *Executor) CancelForOrder(ctx context.Context, orderID uuid.UUID) []Outcome {
	if e == nil || e.store == nil {
		return nil
	}
	shipments, err := e.store.ListShipmentsByOrderID(ctx, orderID)
	if err != nil {
		e.warn("shipmentcancel: list shipments failed", "order_id", orderID.String(), "err", err)
		return nil
	}
	outcomes := make([]Outcome, 0, len(shipments))
	for i := range shipments {
		outcomes = append(outcomes, e.resolveAndExecute(ctx, &shipments[i]))
	}
	return outcomes
}

// CancelShipmentByID drives one shipment (the manual button). Returns an error
// only when the shipment can't be loaded, so the endpoint can 404; the carrier
// outcome itself is in the returned Outcome.
func (e *Executor) CancelShipmentByID(ctx context.Context, shipmentID uuid.UUID) (Outcome, error) {
	sh, err := e.store.GetShipmentByID(ctx, shipmentID)
	if err != nil {
		return Outcome{}, err
	}
	return e.resolveAndExecute(ctx, sh), nil
}

func (e *Executor) resolveAndExecute(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	action := ResolveAction(sh.Status)

	// Idempotent: a shipment already cancelled succeeds again as a no-op so
	// the paid-cancel + coordinator paths (or a manual retry) can't double-hit
	// the carrier. Failed rows are re-attempted so a manual retry can recover.
	if sh.CancelStatus == statusSucceeded {
		return Outcome{ShipmentID: sh.ID, Action: Action(sh.CancelAction), Status: statusSucceeded}
	}

	switch action {
	case ActionNoop:
		return Outcome{ShipmentID: sh.ID, Action: ActionNoop, Status: statusNone}

	case ActionCancelForward:
		return e.execCancelForward(ctx, sh)

	case ActionTriggerRTO:
		return e.execReturnToOrigin(ctx, sh)

	case ActionReversePickup:
		return e.execReversePickup(ctx, sh)

	default:
		return Outcome{ShipmentID: sh.ID, Action: ActionNoop, Status: statusNone}
	}
}

// resolveCarrier validates the tracking number and builds the carrier for a
// shipment. A non-empty failureReason means the caller should record a
// `failed` outcome with it and stop.
func (e *Executor) resolveCarrier(ctx context.Context, sh *shipping.ShipmentRecord) (carrier shipping.Carrier, failureReason string) {
	if strings.TrimSpace(sh.TrackingNumber) == "" {
		return nil, "No tracking number on the shipment — nothing to send to the carrier."
	}
	c, err := e.resolve(ctx, sh.StoreID, sh.Carrier)
	if err != nil {
		e.warn("shipmentcancel: resolve carrier failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		return nil, "Could not reach the carrier — retry from the shipment."
	}
	return c, ""
}

func (e *Executor) execCancelForward(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	carrier, failReason := e.resolveCarrier(ctx, sh)
	if failReason != "" {
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, failReason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: failReason}
	}
	if err := carrier.CancelShipment(ctx, sh.TrackingNumber); err != nil {
		reason := cleanReason(err)
		e.warn("shipmentcancel: carrier cancel failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: reason}
	}
	e.record(ctx, sh.ID, ActionCancelForward, statusSucceeded, "")
	return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusSucceeded}
}

// execReturnToOrigin returns an in-transit shipment to origin. Carriers that
// don't implement ReturnToOriginer record an `unsupported` outcome (the manual
// notice); a carrier rejection (e.g. a state that can't RTO) records `failed`
// with the carrier's clean reason — also a manual notice.
func (e *Executor) execReturnToOrigin(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	carrier, failReason := e.resolveCarrier(ctx, sh)
	if failReason != "" {
		e.record(ctx, sh.ID, ActionTriggerRTO, statusFailed, failReason)
		return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusFailed, Reason: failReason}
	}
	rtoer, ok := carrier.(shipping.ReturnToOriginer)
	if !ok {
		reason := "This carrier can't return an in-transit shipment automatically — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionTriggerRTO, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusUnsupported, Reason: reason}
	}
	if err := rtoer.ReturnToOrigin(ctx, sh.TrackingNumber); err != nil {
		reason := cleanReason(err)
		e.warn("shipmentcancel: carrier RTO failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionTriggerRTO, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusFailed, Reason: reason}
	}
	e.record(ctx, sh.ID, ActionTriggerRTO, statusSucceeded, "")
	return Outcome{ShipmentID: sh.ID, Action: ActionTriggerRTO, Status: statusSucceeded}
}

// execReversePickup creates a reverse (return) pickup for a delivered shipment
// and inserts a new reverse-leg row. Gated by the reversePickupEnabled kill
// switch. Carriers without the capability, or a disabled flag, record
// `unsupported` (the manual notice); a carrier rejection records `failed`.
func (e *Executor) execReversePickup(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	if !e.reversePickupEnabled {
		reason := "Automatic reverse pickup is turned off — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionReversePickup, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusUnsupported, Reason: reason}
	}
	carrier, failReason := e.resolveCarrier(ctx, sh)
	if failReason != "" {
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, failReason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: failReason}
	}
	creator, ok := carrier.(shipping.ReverseShipmentCreator)
	if !ok {
		reason := "This carrier can't create a reverse pickup automatically — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, ActionReversePickup, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusUnsupported, Reason: reason}
	}
	// Reverse the stored addresses: pickup FROM the customer (forward ship_to),
	// return TO the warehouse (forward ship_from).
	warehouse, err := parseShipmentAddress(sh.ShipFrom)
	if err != nil {
		reason := "Could not read the shipment's warehouse address — arrange the return manually."
		e.warn("shipmentcancel: parse ship_from failed", "shipment_id", sh.ID.String(), "err", err)
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: reason}
	}
	customer, err := parseShipmentAddress(sh.ShipTo)
	if err != nil {
		reason := "Could not read the shipment's delivery address — arrange the return manually."
		e.warn("shipmentcancel: parse ship_to failed", "shipment_id", sh.ID.String(), "err", err)
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: reason}
	}
	rev, err := creator.CreateReverseShipment(ctx, shipping.ReverseShipmentRequest{
		OrderID:                sh.OrderID.String(),
		PickupFrom:             customer,
		ReturnTo:               warehouse,
		WarehouseName:          warehouse.Name,
		CurrencyCode:           sh.CurrencyCode,
		OriginalTrackingNumber: sh.TrackingNumber,
	})
	if err != nil {
		reason := cleanReason(err)
		e.warn("shipmentcancel: reverse pickup failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionReversePickup, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusFailed, Reason: reason}
	}
	// Insert the reverse leg as a new shipment row. Its addresses are swapped
	// (pickup from customer → return to warehouse). Marked cancel_action/status
	// so a repeat CancelForOrder skips it (it is itself a reverse action, not a
	// forward shipment to be cancelled). A DB failure here doesn't undo the
	// carrier pickup that already succeeded — log and still record success.
	revRec := &shipping.ShipmentRecord{
		TenantID:       sh.TenantID,
		StoreID:        sh.StoreID,
		OrderID:        sh.OrderID,
		Carrier:        sh.Carrier,
		TrackingNumber: rev.TrackingNumber,
		Status:         "pending",
		ShipFrom:       sh.ShipTo,   // reverse origin = customer
		ShipTo:         sh.ShipFrom, // reverse destination = warehouse
		CurrencyCode:   sh.CurrencyCode,
		CancelAction:   string(ActionReversePickup),
		CancelStatus:   statusSucceeded,
	}
	if err := e.store.CreateShipment(ctx, revRec); err != nil {
		e.warn("shipmentcancel: persist reverse-leg row failed", "shipment_id", sh.ID.String(), "reverse_waybill", rev.TrackingNumber, "err", err)
	}
	reason := "Reverse pickup created: " + rev.TrackingNumber
	e.record(ctx, sh.ID, ActionReversePickup, statusSucceeded, reason)
	return Outcome{ShipmentID: sh.ID, Action: ActionReversePickup, Status: statusSucceeded, Reason: reason}
}

// parseShipmentAddress decodes a shipments.ship_from/ship_to JSONB blob into a
// carrier Address. The keys match what handlers/admin/shipments.go writes on
// label create.
func parseShipmentAddress(raw []byte) (shipping.Address, error) {
	if len(raw) == 0 {
		return shipping.Address{}, errEmptyAddress
	}
	var a struct {
		Name        string `json:"name"`
		Line1       string `json:"line1"`
		Line2       string `json:"line2"`
		City        string `json:"city"`
		Region      string `json:"region"`
		PostalCode  string `json:"postal_code"`
		CountryCode string `json:"country_code"`
		Phone       string `json:"phone"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return shipping.Address{}, err
	}
	return shipping.Address{
		Name: a.Name, Line1: a.Line1, Line2: a.Line2, City: a.City,
		Region: a.Region, PostalCode: a.PostalCode, CountryCode: a.CountryCode, Phone: a.Phone,
	}, nil
}

func (e *Executor) record(ctx context.Context, id uuid.UUID, action Action, status, reason string) {
	if err := e.store.SetShipmentCancelState(ctx, id, string(action), status, reason); err != nil {
		e.warn("shipmentcancel: record cancel state failed", "shipment_id", id.String(), "err", err)
	}
}

func (e *Executor) warn(msg string, args ...any) {
	if e.logger != nil {
		e.logger.Warn(msg, args...)
	}
}

// cleanReason strips the internal "delhivery: cancel shipment: " prefix the
// carrier error carries so the admin sees just the short reason. The carrier
// layer already guarantees no raw body / address leaks (see delhivery.go), so
// this only tidies presentation.
func cleanReason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 <= len(msg) {
		return strings.TrimSpace(msg[i+2:])
	}
	return msg
}
