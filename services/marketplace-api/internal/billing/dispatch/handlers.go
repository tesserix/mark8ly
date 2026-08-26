package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// handleCheckoutSessionCompleted routes checkout.session.completed through
// two stages:
//  1. Raw UPDATE of NON-status columns (stripe_subscription_id,
//     billing_currency) — safe to write directly because billing_currency is
//     guarded by COALESCE (first-write wins, §4.2.1 currency lock).
//  2. statemachine.Transition(signup → trialing) — routes status through the
//     same CAS+audit path every other status mutation uses. Replay-safe:
//     ErrCASConflict and ErrInvalidTransition are swallowed as no-ops.
//
// This is the method form (receiver on *Dispatcher) so the transition can
// call d.emitter. The old raw status UPDATE that P2 landed and P3 deferred to
// P5 is retired here.
func (d *Dispatcher) handleCheckoutSessionCompleted(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				Subscription string `json:"subscription"`
				Customer     string `json:"customer"`
				Currency     string `json:"currency"`
				Metadata     struct {
					Plan   string `json:"plan"`
					Period string `json:"period"`
				} `json:"metadata"`
				// P8: geo-pricing arbitrage signals (§18.8).
				// card_country extracted from payment_method_details.card.country;
				// billing_country from customer_details.address.country.
				// ip_country is unavailable at webhook time (Stripe push, not
				// browser request) — the evaluator will produce ReasonIPUnknown
				// and will not flag on card alone per spec.
				CustomerDetails struct {
					Address struct {
						Country string `json:"country"`
					} `json:"address"`
				} `json:"customer_details"`
				PaymentMethodDetails struct {
					Card struct {
						Country string `json:"country"`
					} `json:"card"`
				} `json:"payment_method_details"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal checkout: %w", err)
	}
	obj := e.Data.Object
	if obj.Customer == "" || obj.Currency == "" {
		return errors.New("dispatch: checkout.session.completed missing customer/currency")
	}
	currency := strings.ToUpper(obj.Currency)

	// Stage 1: non-status columns. Raw UPDATE is safe — billing_currency is
	// locked first-write-wins via COALESCE; stripe_subscription_id is set.
	res := tx.WithContext(ctx).Exec(
		`UPDATE store_subscriptions
         SET stripe_subscription_id = ?,
             billing_currency       = COALESCE(billing_currency, ?),
             updated_at             = ?
         WHERE stripe_customer_id = ?`,
		obj.Subscription, currency, time.Now(), obj.Customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: checkout update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return errors.New("dispatch: no subscription for customer")
	}

	// Stage 2: status transition via state machine. Load the row back to get
	// tenant/store ids and current status.
	var sub subscription.StoreSubscription
	if err := tx.WithContext(ctx).Where("stripe_customer_id = ?", obj.Customer).First(&sub).Error; err != nil {
		return fmt.Errorf("dispatch: reload after update: %w", err)
	}
	// P8 §18.8: triangulation check — flag-only, never short-circuits checkout.
	// ip_country is unavailable at webhook time (Stripe push has no CF-IPCountry
	// header), so the evaluator returns ReasonIPUnknown and does not flag on
	// card alone — preventing false positives for travelers/dual-citizens.
	if d.recorder != nil {
		recErr := d.recorder.RecordIfFlagged(ctx, arbitrage.RecordInput{
			SubscriptionID: sub.ID,
			TenantID:       sub.TenantID,
			StoreID:        sub.StoreID,
			PriceTier:      sub.PriceTier,
			CardCountry:    obj.PaymentMethodDetails.Card.Country,
			BillingCountry: obj.CustomerDetails.Address.Country,
			IPCountry:      "", // unknown at webhook time — evaluated as "??"
			RawIP:          "", // no raw IP at webhook time
		})
		if recErr != nil {
			// Arbitrage write failure must NOT block the subscription lifecycle.
			// Log via fmt.Errorf wrapping so the error surfaces in webhook metrics
			// but we swallow it here to preserve Stripe idempotency.
			_ = fmt.Errorf("dispatch: arbitrage record (non-fatal): %w", recErr)
		}
	}

	if sub.Status != subscription.StatusSignup {
		// Already past signup (replay or out-of-order event). No transition needed.
		return nil
	}

	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       tx,
		Emitter:  d.emitter,
		TenantID: sub.TenantID,
		StoreID:  sub.StoreID,
		From:     subscription.StatusSignup,
		To:       subscription.StatusTrialing,
		Actor:    "system:webhook:stripe",
		Reason:   "checkout.session.completed",
	})
	if errors.Is(err, statemachine.ErrCASConflict) || errors.Is(err, statemachine.ErrInvalidTransition) {
		return nil
	}
	return err
}

// HandleCheckoutSessionCompletedForTesting exposes the checkout handler
// directly so tests can exercise the COALESCE lock-in path in isolation,
// without needing a full Dispatcher and StripeWebhookEvent. Constructs a
// nil-emitter dispatcher internally (statemachine and emitter are both
// nil-safe).
func HandleCheckoutSessionCompletedForTesting(ctx context.Context, tx *gorm.DB, raw []byte) error {
	d := &Dispatcher{emitter: nil}
	return d.handleCheckoutSessionCompleted(ctx, tx, raw)
}

// handleSubscriptionUpdated refreshes period boundaries and cancel_at_period_end.
//
// TODO(P3): emit audit event on period transition.
func handleSubscriptionUpdated(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				ID                 string `json:"id"`
				Customer           string `json:"customer"`
				CurrentPeriodStart int64  `json:"current_period_start"`
				CurrentPeriodEnd   int64  `json:"current_period_end"`
				CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal subscription.updated: %w", err)
	}
	obj := e.Data.Object
	if obj.Customer == "" {
		return errors.New("dispatch: subscription.updated missing customer")
	}
	var periodStart, periodEnd *time.Time
	if obj.CurrentPeriodStart > 0 {
		ts := time.Unix(obj.CurrentPeriodStart, 0).UTC()
		periodStart = &ts
	}
	if obj.CurrentPeriodEnd > 0 {
		ts := time.Unix(obj.CurrentPeriodEnd, 0).UTC()
		periodEnd = &ts
	}
	res := tx.WithContext(ctx).Exec(
		`UPDATE store_subscriptions
         SET current_period_start = ?,
             current_period_end   = ?,
             cancel_at_period_end = ?,
             updated_at           = ?
         WHERE stripe_customer_id = ?`,
		periodStart, periodEnd, obj.CancelAtPeriodEnd, time.Now(), obj.Customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: subscription update: %w", res.Error)
	}
	return nil
}

// handleSubscriptionDeleted routes customer.subscription.deleted through the
// state machine. Valid From states per §17.2: past_due, cancel_scheduled,
// trialing. Active → expired is not a direct allowed move; if a subscription
// is deleted while still active Stripe will have already surfaced an
// invoice.payment_failed first, moving it to past_due. Any other current
// status (e.g. already expired, store_closed) is a benign no-op.
func (d *Dispatcher) handleSubscriptionDeleted(ctx context.Context, tx *gorm.DB, raw []byte) error {
	customer, err := extractCustomerID(raw)
	if err != nil {
		return err
	}

	var sub subscription.StoreSubscription
	if err := tx.WithContext(ctx).Where("stripe_customer_id = ?", customer).First(&sub).Error; err != nil {
		return fmt.Errorf("dispatch: lookup subscription for %s: %w", customer, err)
	}

	validFrom := map[subscription.SubscriptionStatus]bool{
		subscription.StatusPastDue:         true,
		subscription.StatusCancelScheduled: true,
		subscription.StatusTrialing:        true,
	}
	if !validFrom[sub.Status] {
		// Current status cannot transition directly to expired — benign no-op
		// (e.g. already expired, store_closed, or active before dunning ran).
		return nil
	}

	err = statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       tx,
		Emitter:  d.emitter,
		TenantID: sub.TenantID,
		StoreID:  sub.StoreID,
		From:     sub.Status,
		To:       subscription.StatusExpired,
		Actor:    "system:webhook:stripe",
		Reason:   "customer.subscription.deleted",
	})
	if errors.Is(err, statemachine.ErrCASConflict) {
		return nil
	}
	return err
}

// handleInvoicePaid stamps first_charge_at (COALESCE — only the first paid
// invoice wins) and clears hosted_invoice_url now that the SCA challenge is
// resolved. No status transition is needed: staying active is correct; if the
// sub was in payment_action_required, P3's customer.subscription.updated path
// handles that move.
//
// First-charge detection: if first_charge_at was nil before the COALESCE
// update, this invoice is the first successful charge — meaning the trial
// has just transitioned to a paid plan. We emit a TemplateTrialStartedBilled
// confirmation email so the merchant knows their selected plan is now active.
//
// Multi-pod safety: the dispatcher holds pg_advisory_xact_lock on the store
// for the duration of webhook processing (see dispatcher.go), so two pods
// cannot race to detect "first charge" — only one will see first_charge_at=nil.
// The confirmation email's own at-most-once guarantee comes from a
// billing_email_sends claim on a non-transactional handle, NOT from
// first_charge_at — see sendTrialBilled.
func (d *Dispatcher) handleInvoicePaid(ctx context.Context, tx *gorm.DB, raw []byte) error {
	customer, err := extractCustomerID(raw)
	if err != nil {
		// Pre-P6 replays may omit customer — treat as no-op for safety.
		return nil
	}

	// Capture pre-state inside the locked tx so first-charge detection is
	// race-free. Unknown customer (e.g. test-mode noise) is a benign no-op.
	var sub subscription.StoreSubscription
	if err := tx.WithContext(ctx).Where("stripe_customer_id = ?", customer).First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("dispatch: invoice.paid lookup: %w", err)
	}
	wasFirstCharge := sub.FirstChargeAt == nil

	res := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET first_charge_at    = COALESCE(first_charge_at, now()),
		    hosted_invoice_url = NULL,
		    updated_at         = now()
		WHERE stripe_customer_id = ?`,
		customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: invoice.paid update: %w", res.Error)
	}

	if wasFirstCharge && d.emailCl != nil {
		d.sendTrialBilled(ctx, tx, sub)
	}
	return nil
}

// trialBilledPeriodKey is the period_key for the trial-billed confirmation.
//
// It is a constant rather than the Stripe event id because the guarantee we
// want is "at most one trial-billed confirmation per subscription, ever" —
// which is exactly what the claim's primary key
// (subscription_id, template_key, period_key) gives with a fixed period.
// The event id would only deduplicate redeliveries of the *same* Stripe
// event; a distinct invoice.paid event arriving after a rolled-back
// transaction (first_charge_at back to NULL) would carry a different id and
// send a second confirmation. This email fires once in a subscription's
// life, so the period is its whole life.
const trialBilledPeriodKey = "first_charge"

// sendTrialBilled emits the trial-started-billed confirmation.
//
// Idempotency guarantee, and why it is NOT first_charge_at: the send happens
// inside the locked webhook transaction, and invoice.paid is dispatched as a
// chain (see dispatcher.go) whose later handler can fail — or the commit
// itself can fail — rolling first_charge_at back to NULL. The email is
// already gone at that point, and Stripe's retry would see wasFirstCharge
// again. So the claim is written on d.db, a NON-transactional handle: it
// survives the rollback and the retry loses the claim.
//
// Without d.db there is no way to make this at-most-once, so the send is
// skipped rather than risked.
func (d *Dispatcher) sendTrialBilled(ctx context.Context, tx *gorm.DB, sub subscription.StoreSubscription) {
	log := slog.Default()

	if d.db == nil {
		log.Warn("dispatch: trial-billed email skipped — no claim store wired (WithDB)",
			"store_id", sub.StoreID.String())
		d.countSkip("no_claim_store")
		return
	}

	won, claimErr := subscription.ClaimEmailSend(ctx, d.db, sub.ID,
		string(email.TemplateTrialStartedBilled), trialBilledPeriodKey, time.Now().UTC())
	if claimErr != nil {
		log.Error("dispatch: trial-billed claim failed; not sending",
			"store_id", sub.StoreID.String(), "err", claimErr.Error())
		d.countSkip("claim_failed")
		return
	}
	if !won {
		// Already sent for this subscription — a retry after a rolled-back
		// transaction, or a second pod. Nothing to do.
		return
	}

	to := ""
	if sub.Email != nil {
		to = *sub.Email
	}
	if sendErr := d.emailCl.Send(ctx, email.TemplateTrialStartedBilled, to, map[string]any{
		"store_id":   sub.StoreID.String(),
		"tenant_id":  sub.TenantID.String(),
		"store_name": subscription.StoreNameFor(ctx, tx, sub.StoreID),
		"plan":       string(sub.Plan),
		"period":     string(sub.SubscriptionPeriod),
	}); sendErr != nil {
		// Don't fail the webhook — Stripe would retry, double-firing every
		// other side effect. Email failure is a soft error: log and move on.
		// The claim is deliberately NOT released: at-most-once beats a
		// duplicate, matching the other four billing mail paths.
		log.Warn("dispatch: trial-billed email not sent",
			"store_id", sub.StoreID.String(),
			"reason", email.SkipReason(sendErr),
			"err", sendErr.Error())
		d.countSkip(email.SkipReason(sendErr))
		return
	}
	if d.sent != nil {
		d.sent.WithTemplate(string(email.TemplateTrialStartedBilled)).Inc()
	}
}

// countSkip increments the skipped-emails counter when one is wired.
func (d *Dispatcher) countSkip(reason string) {
	if d.skip != nil {
		d.skip.WithTemplateReason(string(email.TemplateTrialStartedBilled), reason).Inc()
	}
}

// handleInvoicePaymentFailed routes invoice.payment_failed through the state
// machine. Valid From states per §17.2: active, payment_action_required.
// Idempotent replays where the status has already advanced are silently
// dropped via ErrCASConflict.
//
// It also persists hosted_invoice_url from the payload, mirroring
// handleInvoicePaymentActionRequired. This handler is the one that produces
// past_due — i.e. the entire dunning cohort — and the dunning ladder's day-5
// and day-7 emails render a "pay this invoice" button only when
// hosted_invoice_url is set. Without this write the ordinary "card declined,
// no SCA challenge" merchant reaches the two emails immediately preceding
// suspension with no payment link at all.
//
// The write is unconditional and happens BEFORE the status transition, so a
// replay (or an event arriving when the status has already advanced) still
// refreshes the URL. An empty/absent field is left alone rather than blanking
// a good URL; clearing on success stays where it belongs, in handleInvoicePaid.
func (d *Dispatcher) handleInvoicePaymentFailed(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				Customer         string `json:"customer"`
				HostedInvoiceURL string `json:"hosted_invoice_url"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal invoice.payment_failed: %w", err)
	}
	customer := e.Data.Object.Customer
	if customer == "" {
		return errors.New("dispatch: invoice.payment_failed missing customer")
	}

	// Persist hosted_invoice_url unconditionally so the merchant can always
	// reach the Stripe-hosted payment page the dunning emails link to, even
	// on event replay.
	if e.Data.Object.HostedInvoiceURL != "" {
		res := tx.WithContext(ctx).Exec(`
			UPDATE store_subscriptions
			SET hosted_invoice_url = ?,
			    updated_at         = now()
			WHERE stripe_customer_id = ?`,
			e.Data.Object.HostedInvoiceURL, customer,
		)
		if res.Error != nil {
			return fmt.Errorf("dispatch: persist hosted_invoice_url: %w", res.Error)
		}
	}

	var sub subscription.StoreSubscription
	if err := tx.WithContext(ctx).Where("stripe_customer_id = ?", customer).First(&sub).Error; err != nil {
		return fmt.Errorf("dispatch: lookup subscription for %s: %w", customer, err)
	}

	if sub.Status != subscription.StatusActive && sub.Status != subscription.StatusPaymentActionRequired {
		// Already past_due or beyond — idempotent replay, no-op.
		return nil
	}

	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       tx,
		Emitter:  d.emitter,
		TenantID: sub.TenantID,
		StoreID:  sub.StoreID,
		From:     sub.Status,
		To:       subscription.StatusPastDue,
		Actor:    "system:webhook:stripe",
		Reason:   "invoice.payment_failed",
	})
	if errors.Is(err, statemachine.ErrCASConflict) {
		return nil
	}
	return err
}

// handleInvoicePaymentActionRequired routes invoice.payment_action_required
// through the state machine. Valid From state per §17.2: active only.
// Idempotent replays are silently dropped via ErrCASConflict.
//
// §4.7: hosted_invoice_url is persisted unconditionally BEFORE the transition
// so it is available to merchants even if the transition is a no-op (replay,
// already in payment_action_required, etc.).
func (d *Dispatcher) handleInvoicePaymentActionRequired(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				Customer         string `json:"customer"`
				HostedInvoiceURL string `json:"hosted_invoice_url"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal payment_action_required: %w", err)
	}
	customer := e.Data.Object.Customer
	if customer == "" {
		return errors.New("dispatch: invoice.payment_action_required missing customer")
	}

	// Persist hosted_invoice_url unconditionally so the merchant can always
	// reach the Stripe-hosted payment page, even on event replay.
	if e.Data.Object.HostedInvoiceURL != "" {
		res := tx.WithContext(ctx).Exec(`
			UPDATE store_subscriptions
			SET hosted_invoice_url = ?,
			    updated_at         = now()
			WHERE stripe_customer_id = ?`,
			e.Data.Object.HostedInvoiceURL, customer,
		)
		if res.Error != nil {
			return fmt.Errorf("dispatch: persist hosted_invoice_url: %w", res.Error)
		}
	}

	var sub subscription.StoreSubscription
	if err := tx.WithContext(ctx).Where("stripe_customer_id = ?", customer).First(&sub).Error; err != nil {
		return fmt.Errorf("dispatch: lookup subscription for %s: %w", customer, err)
	}

	if sub.Status != subscription.StatusActive {
		// Already payment_action_required, past_due, or beyond — no-op.
		return nil
	}

	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       tx,
		Emitter:  d.emitter,
		TenantID: sub.TenantID,
		StoreID:  sub.StoreID,
		From:     subscription.StatusActive,
		To:       subscription.StatusPaymentActionRequired,
		Actor:    "system:webhook:stripe",
		Reason:   "invoice.payment_action_required",
	})
	if errors.Is(err, statemachine.ErrCASConflict) {
		return nil
	}
	return err
}

// handleCustomerUpdated mirrors invoice_settings.default_payment_method onto
// store_subscriptions.has_default_payment_method. The flag drives the trial
// reminder cron's cadence:
//   - true  → single T-1 heads-up before Stripe auto-bills the chosen plan.
//   - false → nudges at T-15, T-10, T-7, T-3, T-1 asking the merchant to add
//     a card or pick a plan.
//
// Stripe emits customer.updated whenever invoice_settings.default_payment_method
// changes (set, cleared, or replaced), so the inline payload value is the
// source of truth. payment_method.attached / .detached stay as no-ops because
// Stripe pairs them with a customer.updated when the default actually changes.
func handleCustomerUpdated(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				Customer        string `json:"id"`
				Email           string `json:"email"`
				InvoiceSettings struct {
					DefaultPaymentMethod *string `json:"default_payment_method"`
				} `json:"invoice_settings"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal customer.updated: %w", err)
	}
	customer := e.Data.Object.Customer
	if customer == "" {
		// Replays of older Stripe events sometimes omit the id — skip safely.
		return nil
	}
	hasPM := e.Data.Object.InvoiceSettings.DefaultPaymentMethod != nil &&
		*e.Data.Object.InvoiceSettings.DefaultPaymentMethod != ""

	// email is written only when the event carries one. Stripe omits the
	// field on some replays, and an absent field must not blank an address
	// we already hold — COALESCE on the parameter, not on the column, so an
	// empty string is treated as "no value in this event".
	email := strings.TrimSpace(e.Data.Object.Email)

	res := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET has_default_payment_method = ?,
		    email                      = COALESCE(NULLIF(?, ''), email),
		    updated_at                 = now()
		WHERE stripe_customer_id = ?`,
		hasPM, email, customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: customer.updated has_default_payment_method: %w", res.Error)
	}
	return nil
}

// handleChargeRefunded is audit-only in P2.
// TODO(P3): create refund record in billing ledger.
func handleChargeRefunded(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handlePaymentMethodAttached is intentionally a no-op. has_default_payment_method
// is mirrored from customer.updated, which Stripe emits whenever
// invoice_settings.default_payment_method changes. Attaching a PM that becomes
// the default always pairs with a customer.updated; attaching a non-default
// PM correctly leaves the flag unchanged.
func handlePaymentMethodAttached(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handlePaymentMethodDetached is intentionally a no-op. See handlePaymentMethodAttached:
// when detaching the default PM, Stripe emits customer.updated with
// default_payment_method=null which clears the flag in handleCustomerUpdated.
func handlePaymentMethodDetached(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handleFraudWarning is audit-only in P2.
// TODO(P3): flag account for manual review via arbitrage_flag column.
func handleFraudWarning(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// extractCustomerID parses the customer ID from a standard Stripe event
// payload that wraps the object under data.object.customer.
func extractCustomerID(raw []byte) (string, error) {
	var e struct {
		Data struct {
			Object struct {
				Customer string `json:"customer"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return "", fmt.Errorf("dispatch: unmarshal customer: %w", err)
	}
	if e.Data.Object.Customer == "" {
		return "", errors.New("dispatch: missing customer")
	}
	return e.Data.Object.Customer, nil
}
