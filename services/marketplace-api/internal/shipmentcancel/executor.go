package shipmentcancel

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// ShipmentStore is the narrow persistence surface the executor needs.
// shipping.Repository satisfies it.
type ShipmentStore interface {
	ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]shipping.ShipmentRecord, error)
	GetShipmentByID(ctx context.Context, id uuid.UUID) (*shipping.ShipmentRecord, error)
	SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error
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
	store   ShipmentStore
	resolve CarrierResolver
	logger  *slog.Logger
}

// NewExecutor constructs an Executor. logger may be nil.
func NewExecutor(store ShipmentStore, resolve CarrierResolver, logger *slog.Logger) *Executor {
	return &Executor{store: store, resolve: resolve, logger: logger}
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

	case ActionTriggerRTO, ActionReversePickup:
		// Phase 2/3 handle these; until then, record so the admin sees it and
		// arranges the return manually with the carrier.
		reason := "This shipment has left for delivery — arrange the return manually with the carrier."
		e.record(ctx, sh.ID, action, statusUnsupported, reason)
		return Outcome{ShipmentID: sh.ID, Action: action, Status: statusUnsupported, Reason: reason}

	default:
		return Outcome{ShipmentID: sh.ID, Action: ActionNoop, Status: statusNone}
	}
}

func (e *Executor) execCancelForward(ctx context.Context, sh *shipping.ShipmentRecord) Outcome {
	if strings.TrimSpace(sh.TrackingNumber) == "" {
		reason := "No tracking number on the shipment — nothing to cancel with the carrier."
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: reason}
	}
	carrier, err := e.resolve(ctx, sh.StoreID, sh.Carrier)
	if err != nil {
		reason := "Could not reach the carrier to cancel — retry from the shipment."
		e.warn("shipmentcancel: resolve carrier failed", "shipment_id", sh.ID.String(), "carrier", sh.Carrier, "err", err)
		e.record(ctx, sh.ID, ActionCancelForward, statusFailed, reason)
		return Outcome{ShipmentID: sh.ID, Action: ActionCancelForward, Status: statusFailed, Reason: reason}
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
