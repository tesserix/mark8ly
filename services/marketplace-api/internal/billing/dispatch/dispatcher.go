// Package dispatch routes Stripe webhook events to per-type handlers that
// perform minimal column updates on store_subscriptions and emit audit events.
// Full state-machine transitions land in P3.
package dispatch

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// Handler is a function that processes a single Stripe webhook event type.
// It receives the locked transaction handle and the raw JSON payload.
type Handler func(ctx context.Context, tx *gorm.DB, raw []byte) error

// Dispatcher routes incoming webhook events to registered per-type handlers.
type Dispatcher struct {
	handlers map[string]Handler
}

// New returns a Dispatcher with the P2 allowlist wired. P3 will add richer
// handlers around state transitions; for now each handler mutates only the
// columns listed in its doc comment.
func New() *Dispatcher {
	d := &Dispatcher{handlers: map[string]Handler{}}
	d.handlers["checkout.session.completed"] = handleCheckoutSessionCompleted
	d.handlers["customer.subscription.updated"] = handleSubscriptionUpdated
	d.handlers["customer.subscription.deleted"] = handleSubscriptionDeleted
	d.handlers["invoice.paid"] = handleInvoicePaid
	d.handlers["invoice.payment_failed"] = handleInvoicePaymentFailed
	d.handlers["invoice.payment_action_required"] = handleInvoicePaymentActionRequired
	d.handlers["customer.updated"] = handleCustomerUpdated
	d.handlers["charge.refunded"] = handleChargeRefunded
	d.handlers["payment_method.attached"] = handlePaymentMethodAttached
	d.handlers["payment_method.detached"] = handlePaymentMethodDetached
	d.handlers["radar.early_fraud_warning"] = handleFraudWarning
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
