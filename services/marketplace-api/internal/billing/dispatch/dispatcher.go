// Package dispatch routes Stripe webhook events to per-type handlers that
// perform minimal column updates on store_subscriptions and emit audit events.
package dispatch

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// Handler is a function that processes a single Stripe webhook event type.
// It receives the locked transaction handle and the raw JSON payload.
type Handler func(ctx context.Context, tx *gorm.DB, raw []byte) error

// Dispatcher routes incoming webhook events to registered per-type handlers.
type Dispatcher struct {
	emitter  *audit.Emitter
	handlers map[string]Handler
}

// New returns a Dispatcher with all P2/P3 handlers wired. em may be nil for
// tests that opt out of audit emission — Emitter.EmitStateTransition is a
// no-op on a nil receiver.
func New(em *audit.Emitter) *Dispatcher {
	d := &Dispatcher{emitter: em, handlers: map[string]Handler{}}
	// Free functions — unchanged from P2.
	d.handlers["checkout.session.completed"] = handleCheckoutSessionCompleted
	d.handlers["customer.subscription.updated"] = handleSubscriptionUpdated
	d.handlers["invoice.paid"] = handleInvoicePaid
	d.handlers["customer.updated"] = handleCustomerUpdated
	d.handlers["charge.refunded"] = handleChargeRefunded
	d.handlers["payment_method.attached"] = handlePaymentMethodAttached
	d.handlers["payment_method.detached"] = handlePaymentMethodDetached
	d.handlers["radar.early_fraud_warning"] = handleFraudWarning
	// Methods — state mutations routed through statemachine.Transition.
	d.handlers["customer.subscription.deleted"] = d.handleSubscriptionDeleted
	d.handlers["invoice.payment_failed"] = d.handleInvoicePaymentFailed
	d.handlers["invoice.payment_action_required"] = d.handleInvoicePaymentActionRequired
	return d
}

// Dispatch routes e to the handler registered for e.EventType. The caller
// already holds pg_advisory_xact_lock on the store, and tx is that locked
// transaction's handle.
func (d *Dispatcher) Dispatch(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
	h, ok := d.handlers[e.EventType]
	if !ok {
		return fmt.Errorf("dispatch: no handler for %s", e.EventType)
	}
	return h(ctx, tx, []byte(e.Payload))
}
