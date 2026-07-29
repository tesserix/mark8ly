package order

import (
	"encoding/json"

	"gorm.io/datatypes"
)

// EventKind is the enumerated set of values stored in order_events.kind.
// Handlers inspect the kind first and unmarshal Payload into the matching
// struct below. Adding a new kind is a two-step process: add the constant
// here AND add the payload builder that produces its JSON.
type EventKind string

const (
	EventKindCreated         EventKind = "created"
	EventKindStatusChanged   EventKind = "status_changed"
	EventKindPaymentRecorded EventKind = "payment_recorded"
	EventKindFulfilled       EventKind = "fulfilled"
	EventKindCancelled       EventKind = "cancelled"
	EventKindRefunded        EventKind = "refunded"
	EventKindReturnRequested EventKind = "return_requested"
	EventKindReturnApproved  EventKind = "return_approved"
	EventKindReturnReceived  EventKind = "return_received"
	EventKindReturnRefunded  EventKind = "return_refunded"
	EventKindReturnRejected  EventKind = "return_rejected"

	// Fulfillment / shipment timeline. These feed the customer-facing
	// order tracking view. `shipment_created` fires when a label is
	// purchased; the *_status kinds fire as the shipment moves through
	// Delhivery/ShipEngine's tracking lifecycle.
	EventKindShipmentCreated        EventKind = "shipment_created"
	EventKindShipmentInTransit      EventKind = "shipment_in_transit"
	EventKindShipmentOutForDelivery EventKind = "shipment_out_for_delivery"
	EventKindShipmentDelivered      EventKind = "shipment_delivered"
	EventKindShipmentException      EventKind = "shipment_exception"
	// EventKindPickupScheduled fires when the carrier has accepted a
	// pickup request for the shipment (either freshly scheduled or
	// recognised as an idempotent duplicate). Appending this to the
	// order timeline lets the customer-facing tracking feed render
	// "Pickup scheduled for Fri, Apr 21" even before the waybill has
	// moved off Manifested.
	EventKindPickupScheduled EventKind = "pickup_scheduled"
	// EventKindPickupFailed fires when auto-scheduling a carrier pickup
	// failed after the label was created (wallet balance, account limits,
	// carrier hiccup). It's an operational signal for the merchant only —
	// the label is still valid and a manual pickup can be scheduled — so
	// it is deliberately EXCLUDED from the customer-facing storefront
	// timeline (see loadTimeline's admin-only kinds). Never surface it to
	// buyers.
	EventKindPickupFailed EventKind = "pickup_failed"
	// EventKindGiftCardCredited records the store-credit half of a refund.
	// orders.refunded_amount only counts gateway money, so without this the
	// portion returned to a gift card is invisible on the order and the
	// refund looks smaller than it was. Customer-facing.
	EventKindGiftCardCredited EventKind = "gift_card_credited"
	// EventKindGiftCardCreditSkipped records store credit that could NOT be
	// returned to a card — most often because the card's own purchase was
	// already refunded, so crediting it would create value from nothing.
	// It is a reconciliation task for the merchant, not something a buyer
	// should read, so it is EXCLUDED from the storefront timeline (see
	// loadTimeline's admin-only kinds).
	EventKindGiftCardCreditSkipped EventKind = "gift_card_credit_skipped"
)

// GiftCardRefundPayload is written for both gift-card refund event kinds.
type GiftCardRefundPayload struct {
	GiftCardID  string `json:"gift_card_id"`
	Amount      string `json:"amount"`
	Description string `json:"description,omitempty"`
}

// EncodeGiftCardRefund produces a datatypes.JSON payload for the gift-card
// refund kinds.
func EncodeGiftCardRefund(p GiftCardRefundPayload) datatypes.JSON { return mustJSON(p) }

// ShipmentEventPayload is written for every shipment lifecycle event.
// Customer UIs can render the tracking timeline off this list alone —
// the Shipment row is joined separately for label/ETA.
type ShipmentEventPayload struct {
	ShipmentID     string `json:"shipment_id"`
	Carrier        string `json:"carrier"`
	TrackingNumber string `json:"tracking_number,omitempty"`
	Status         string `json:"status"` // e.g. "created", "in_transit", "delivered"
	Description    string `json:"description,omitempty"`
}

// EncodeShipment produces a datatypes.JSON payload for shipment kinds.
func EncodeShipment(p ShipmentEventPayload) datatypes.JSON { return mustJSON(p) }

// -----------------------------------------------------------------------------
// Payload shapes. All timestamps are RFC3339 strings when serialized.
// -----------------------------------------------------------------------------

// CreatedPayload is written when an order is first inserted.
type CreatedPayload struct {
	OrderNumber  string `json:"order_number"`
	CurrencyCode string `json:"currency_code"`
	GrandTotal   string `json:"grand_total"`
}

// StatusChangedPayload covers every status axis (order, payment, fulfillment).
// Axis identifies which enum the transition applies to.
type StatusChangedPayload struct {
	Axis   string `json:"axis"` // "order" | "payment" | "fulfillment"
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

// PaymentRecordedPayload is written when a payment state update is accepted.
type PaymentRecordedPayload struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

// RefundedPayload is written alongside the atomic refunded_amount update.
// Amount is a decimal string (matches the DB column type).
type RefundedPayload struct {
	Amount           string `json:"amount"`
	TotalRefunded    string `json:"total_refunded_after"`
	PaymentStatusTo  string `json:"payment_status_to"`
	Reason           string `json:"reason,omitempty"`
}

// ReturnEventPayload covers all return_*_requested|approved|received|refunded|rejected
// events. ReturnID is the primary reference; snapshot fields carry the return
// state at the moment the event was written.
type ReturnEventPayload struct {
	ReturnID     string `json:"return_id"`
	ReturnNumber string `json:"return_number"`
	Amount       string `json:"amount,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// -----------------------------------------------------------------------------
// Encoders. Services call these to produce datatypes.JSON for OrderEvent.Payload.
// Panicking on marshal failure is intentional — these are pure value types with
// no fields that can fail to serialize.
// -----------------------------------------------------------------------------

func mustJSON(v any) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		panic("order: event payload marshal: " + err.Error())
	}
	return datatypes.JSON(b)
}

func EncodeCreated(p CreatedPayload) datatypes.JSON             { return mustJSON(p) }
func EncodeStatusChanged(p StatusChangedPayload) datatypes.JSON { return mustJSON(p) }
func EncodePaymentRecorded(p PaymentRecordedPayload) datatypes.JSON {
	return mustJSON(p)
}
func EncodeRefunded(p RefundedPayload) datatypes.JSON       { return mustJSON(p) }
func EncodeReturnEvent(p ReturnEventPayload) datatypes.JSON { return mustJSON(p) }
