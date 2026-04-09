package order

// OrderStatus is the operational lifecycle of an order. It is ORTHOGONAL to
// PaymentStatus and FulfillmentStatus — 'refunded' is deliberately absent here
// and lives on PaymentStatus only.
//
// See spec §7 and §2 decision 4.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusFulfilled OrderStatus = "fulfilled"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// orderStatusTransitions is the complete set of legal transitions.
// Nil slice ⇒ terminal. Absent key ⇒ illegal starting state.
var orderStatusTransitions = map[OrderStatus][]OrderStatus{
	OrderStatusPending:   {OrderStatusConfirmed, OrderStatusCancelled},
	OrderStatusConfirmed: {OrderStatusFulfilled, OrderStatusCancelled},
	OrderStatusFulfilled: nil, // terminal
	OrderStatusCancelled: nil, // terminal
}

// CanTransitionTo returns true iff target is a legal next state from s.
func (s OrderStatus) CanTransitionTo(target OrderStatus) bool {
	allowed, ok := orderStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

// Allowed returns the legal next states from s (may be empty for terminal).
// Returns a defensive copy so callers cannot mutate the transition table.
func (s OrderStatus) Allowed() []OrderStatus {
	return append([]OrderStatus(nil), orderStatusTransitions[s]...)
}

// -----------------------------------------------------------------------------
// PaymentStatus — independent axis. Carries refund semantics.
// -----------------------------------------------------------------------------

type PaymentStatus string

const (
	PaymentStatusPending            PaymentStatus = "pending"
	PaymentStatusAuthorized         PaymentStatus = "authorized"
	PaymentStatusPaid               PaymentStatus = "paid"
	PaymentStatusFailed             PaymentStatus = "failed"
	PaymentStatusRefunded           PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded  PaymentStatus = "partially_refunded"
)

var paymentStatusTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentStatusPending:           {PaymentStatusAuthorized, PaymentStatusPaid, PaymentStatusFailed},
	PaymentStatusAuthorized:        {PaymentStatusPaid, PaymentStatusFailed, PaymentStatusRefunded},
	PaymentStatusPaid:              {PaymentStatusRefunded, PaymentStatusPartiallyRefunded},
	PaymentStatusPartiallyRefunded: {PaymentStatusRefunded},
	PaymentStatusFailed:            {PaymentStatusPending, PaymentStatusPaid}, // retry path
	PaymentStatusRefunded:          nil,                                        // terminal
}

func (s PaymentStatus) CanTransitionTo(target PaymentStatus) bool {
	allowed, ok := paymentStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

func (s PaymentStatus) Allowed() []PaymentStatus {
	return append([]PaymentStatus(nil), paymentStatusTransitions[s]...)
}

// -----------------------------------------------------------------------------
// FulfillmentStatus — independent axis.
// FulfillmentStatusPartial is forward-compatible only; slice 1 never writes it.
// -----------------------------------------------------------------------------

type FulfillmentStatus string

const (
	FulfillmentStatusUnfulfilled FulfillmentStatus = "unfulfilled"
	FulfillmentStatusPartial     FulfillmentStatus = "partial" // slice 2
	FulfillmentStatusFulfilled   FulfillmentStatus = "fulfilled"
)

var fulfillmentStatusTransitions = map[FulfillmentStatus][]FulfillmentStatus{
	FulfillmentStatusUnfulfilled: {FulfillmentStatusPartial, FulfillmentStatusFulfilled},
	FulfillmentStatusPartial:     {FulfillmentStatusFulfilled},
	FulfillmentStatusFulfilled:   nil, // terminal
}

func (s FulfillmentStatus) CanTransitionTo(target FulfillmentStatus) bool {
	allowed, ok := fulfillmentStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

func (s FulfillmentStatus) Allowed() []FulfillmentStatus {
	return append([]FulfillmentStatus(nil), fulfillmentStatusTransitions[s]...)
}
