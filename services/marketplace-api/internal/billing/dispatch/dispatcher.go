// Package dispatch routes Stripe webhook events to per-type handlers that
// perform minimal column updates on store_subscriptions and emit audit events.
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/appaddon"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// Handler is a function that processes a single Stripe webhook event type.
// It receives the locked transaction handle and the raw JSON payload.
type Handler func(ctx context.Context, tx *gorm.DB, raw []byte) error

// chain runs handlers in order inside the same transaction. Bails on
// first error — used by invoice.paid to sequence the generic handler
// ahead of the P15 sub-handler.
func chain(handlers ...Handler) Handler {
	return func(ctx context.Context, tx *gorm.DB, raw []byte) error {
		for _, h := range handlers {
			if err := h(ctx, tx, raw); err != nil {
				return err
			}
		}
		return nil
	}
}

// Dispatcher routes incoming webhook events to registered per-type handlers.
type Dispatcher struct {
	emitter  *audit.Emitter
	recorder *arbitrage.Recorder // nil-safe: arbitrage check is skipped when nil
	emailCl  email.Client        // nil-safe: trial-billed confirmation email is skipped when nil
	// db is a NON-transactional handle, deliberately separate from the tx
	// passed to Dispatch. The trial-billed claim is taken at drain time,
	// after the webhook transaction has committed (see sendTrialBilled), so
	// tx is long gone by then and this is the only usable handle. Nil
	// disables the trial-billed email entirely — there is no way to make it
	// at-most-once without a claim store.
	db       *gorm.DB
	skip     SkipCounter // nil-safe: skipped-send counting is optional
	sent     SentCounter // nil-safe: delivered-send counting is optional
	handlers map[string]Handler
}

// New returns a Dispatcher with all P2/P3 handlers wired. em may be nil for
// tests that opt out of audit emission — Emitter.EmitStateTransition is a
// no-op on a nil receiver.
func New(em *audit.Emitter) *Dispatcher {
	d := &Dispatcher{emitter: em, handlers: map[string]Handler{}}
	// Side-effect-only handlers that don't advance status. Most are free
	// functions; customer.subscription.updated is a method because it emits
	// a period/cancel-flag transition event through d.emitter (#705).
	d.handlers["customer.subscription.updated"] = d.handleSubscriptionUpdated
	// invoice.paid runs a chain: the generic handler first (stamps
	// first_charge_at, clears hosted_invoice_url, emits trial-billed
	// confirmation email if this is the first charge), then the P15
	// white-label app sub-handler that flips has_white_label_app_add_on
	// when metadata.kind matches. Errors in either stage bail the chain.
	d.handlers["invoice.paid"] = chain(d.handleInvoicePaid, appaddon.HandleInvoicePaidForAppAddOn)
	d.handlers["customer.updated"] = handleCustomerUpdated
	d.handlers["charge.refunded"] = handleChargeRefunded
	d.handlers["payment_method.attached"] = handlePaymentMethodAttached
	d.handlers["payment_method.detached"] = handlePaymentMethodDetached
	d.handlers["radar.early_fraud_warning"] = handleFraudWarning
	// Methods — state mutations routed through statemachine.Transition.
	d.handlers["checkout.session.completed"] = d.handleCheckoutSessionCompleted
	d.handlers["customer.subscription.deleted"] = d.handleSubscriptionDeleted
	d.handlers["invoice.payment_failed"] = d.handleInvoicePaymentFailed
	d.handlers["invoice.payment_action_required"] = d.handleInvoicePaymentActionRequired
	return d
}

// WithRecorder attaches an arbitrage.Recorder to the Dispatcher so that
// checkout.session.completed events trigger the geo-pricing triangulation
// check per spec §18.8. Recorder may be nil (skips the check).
func (d *Dispatcher) WithRecorder(r *arbitrage.Recorder) *Dispatcher {
	d.recorder = r
	return d
}

// CounterIncrementer is a one-method counter so tests can stub it.
type CounterIncrementer interface{ Inc() }

// SkipCounter counts billing emails deliberately not sent, labeled by
// template and reason. Mirrors dunning.SkipCounter and lifecycle.SkipCounter
// so main can feed all three from metrics.BillingEmailsSkippedTotal.
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// SentCounter counts billing emails actually delivered, labeled by template.
type SentCounter interface {
	WithTemplate(template string) CounterIncrementer
}

// WithDB attaches a non-transactional database handle used to claim a
// billing_email_sends row immediately before the trial-billed confirmation is
// sent. It must NOT be the webhook transaction: claim and send both run after
// that transaction has committed, when its handle is no longer usable.
func (d *Dispatcher) WithDB(conn *gorm.DB) *Dispatcher {
	d.db = conn
	return d
}

// WithSkipCounter attaches the counter for trial-billed emails deliberately
// not sent. Optional.
func (d *Dispatcher) WithSkipCounter(c SkipCounter) *Dispatcher {
	d.skip = c
	return d
}

// WithSentCounter attaches the counter for trial-billed emails delivered.
// Optional.
func (d *Dispatcher) WithSentCounter(c SentCounter) *Dispatcher {
	d.sent = c
	return d
}

// WithEmail attaches an email.Client so the dispatcher can emit the
// trial-billed confirmation email on the first successful invoice charge.
// emailCl may be nil — the send is then skipped without affecting other
// invoice.paid side effects. WithDB must be called too; without a claim
// store the send is skipped.
func (d *Dispatcher) WithEmail(c email.Client) *Dispatcher {
	d.emailCl = c
	return d
}

// Dispatch routes e to the handler registered for e.EventType. The caller
// already holds pg_advisory_xact_lock on the store, and tx is that locked
// transaction's handle.
//
// On each invocation the handler latency is observed on
// metrics.Subscription.StripeWebhookProcessingDuration and, when the handler
// returns a non-nil error, metrics.Subscription.StripeWebhookFailedTotal is
// incremented with a classified reason.
func (d *Dispatcher) Dispatch(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
	h, ok := d.handlers[e.EventType]
	if !ok {
		return fmt.Errorf("dispatch: no handler for %s", e.EventType)
	}

	start := time.Now()
	err := h(ctx, tx, []byte(e.Payload))
	durationSecs := time.Since(start).Seconds()

	if metrics.Subscription != nil {
		statusLabel := "ok"
		if err != nil {
			statusLabel = "error"
		}
		metrics.Subscription.StripeWebhookProcessingDuration.
			WithLabelValues(e.EventType, statusLabel).Observe(durationSecs)

		if err != nil {
			metrics.Subscription.StripeWebhookFailedTotal.
				WithLabelValues(e.EventType, classifyWebhookErr(err)).Inc()
		}
	}

	return err
}

// classifyWebhookErr maps a handler error to a short label suitable for the
// stripe_webhook_failed_total counter. The label vocabulary must stay small and
// stable so dashboards can reference fixed values.
func classifyWebhookErr(err error) string {
	if errors.Is(err, statemachine.ErrCASConflict) {
		return "cas_conflict"
	}
	if errors.Is(err, statemachine.ErrInvalidTransition) {
		return "invalid_transition"
	}
	var apiErr *billingstripe.APIError
	if errors.As(err, &apiErr) {
		return "stripe_api"
	}
	// Heuristic: GORM / pgx errors tend to contain "sql" or "pq" in the chain.
	// Use a simple string check rather than importing internal pgx types.
	msg := err.Error()
	if len(msg) > 0 {
		for _, token := range []string{"sql:", "pq:", "ERROR:", "pgconn"} {
			if containsSubstring(msg, token) {
				return "db"
			}
		}
	}
	return "unknown"
}

// containsSubstring is a minimal helper to avoid importing strings in the
// package-level function — kept package-private.
func containsSubstring(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
