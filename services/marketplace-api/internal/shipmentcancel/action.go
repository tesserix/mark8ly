// Package shipmentcancel resolves and executes the carrier action for a
// shipment when its order is refunded or cancelled. The decision layer here
// is pure and carrier-agnostic; the executor (executor.go) dispatches to the
// carrier and records outcomes.
package shipmentcancel

import "strings"

// Action is the carrier action a shipment's current lifecycle state calls for.
type Action string

const (
	// ActionNoop — nothing to do (already cancelled/returned/exception, or an
	// unknown state we deliberately leave for manual handling).
	ActionNoop Action = "none"
	// ActionCancelForward — pre-pickup: cancel the forward waybill.
	ActionCancelForward Action = "cancel_forward"
	// ActionTriggerRTO — in transit: return to origin (Phase 2; unsupported in Phase 1).
	ActionTriggerRTO Action = "rto"
	// ActionReversePickup — delivered: create a reverse pickup (Phase 3; unsupported in Phase 1).
	ActionReversePickup Action = "reverse_pickup"
)

// ResolveAction maps a shipment's status column to the carrier action. Pure:
// no carrier or DB dependency. Unknown/terminal states resolve to ActionNoop
// so we never take a destructive action on a state we don't understand.
func ResolveAction(shipmentStatus string) Action {
	switch strings.ToLower(strings.TrimSpace(shipmentStatus)) {
	case "", "pending", "created", "manifested":
		return ActionCancelForward
	case "in_transit", "out_for_delivery":
		return ActionTriggerRTO
	case "delivered":
		return ActionReversePickup
	default:
		// cancelled, canceled, returned, rto, exception, and anything unknown.
		return ActionNoop
	}
}
