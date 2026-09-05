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
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/postcommit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// arbitrageFailureReason is the stripe_webhook_failed_total reason label used
// when the arbitrage recorder fails on a checkout event. The failure is
// non-fatal to the webhook, so it never reaches Dispatch()'s classifier and has
// to be counted at the call site (#423).
const arbitrageFailureReason = "arbitrage_record"

// arbitrageFailureEventType is the event_type label for the counter above.
// Only checkout.session.completed drives the recorder today.
const arbitrageFailureEventType = "checkout.session.completed"

// reportArbitrageFailure logs and counts a non-fatal arbitrage recorder
// failure. Dispatch() only classifies errors a handler RETURNS, and this one is
// deliberately swallowed, so the counter has to be incremented here or the
// failure is invisible (#423).
//
// Note for whoever picks up #438: today this branch is unreachable in
// production, because checkout.session.completed carries no IP country and
// arbitrage.Evaluate never flags without one. It is still wired — and tested —
// so that moving the recorder call somewhere with an IP signal does not
// silently reintroduce the swallow.
func reportArbitrageFailure(sub subscription.StoreSubscription, recErr error) {
	slog.Default().Error("dispatch: arbitrage record failed (non-fatal)",
		"event_type", arbitrageFailureEventType,
		"subscription_id", sub.ID.String(),
		"tenant_id", sub.TenantID.String(),
		"store_id", sub.StoreID.String(),
		"err", recErr.Error())
	if metrics.Subscription != nil {
		metrics.Subscription.StripeWebhookFailedTotal.
			WithLabelValues(arbitrageFailureEventType, arbitrageFailureReason).Inc()
	}
}

// recordArbitrage hands the arbitrage recorder call to the request's
// post-commit collector so it runs AFTER the webhook's advisory-lock
// transaction commits, instead of inside it.
//
// Why it cannot run inline (#438). The caller is inside
// subscription.WithAdvisoryLock, and handleCheckoutSessionCompleted has
// already run `UPDATE store_subscriptions ... WHERE stripe_customer_id = ?`
// on tx, which holds a FOR NO KEY UPDATE row lock on the subscription row for
// the rest of the transaction. The recorder writes on its own *gorm.DB — the
// pool handle, a DIFFERENT connection — and its step 3 is
// `UPDATE store_subscriptions ... SET arbitrage_flag = true` on that same row.
// The recorder's connection blocks on the uncommitted row lock while the
// transaction that holds it is blocked in Go waiting for the recorder to
// return. Postgres sees one waiter and one idle-in-transaction session: no
// cycle, so deadlock_timeout never fires, and no lock_timeout or
// statement_timeout is configured. The webhook hangs indefinitely.
//
// This is latent today only because the call site hard-codes IPCountry: "" and
// arbitrage.Evaluate never flags without an IP country, so the recorder's
// writes are never reached. The first caller to supply a real IP country arms
// the stall. Deferring the call removes the overlap regardless.
//
// Passing the caller's tx into the recorder would also remove the overlap, but
// it would put the audit row back inside the webhook transaction — undoing
// #423/#442, which deliberately moved it out so a failed flag toggle cannot
// destroy the fraud record.
//
// Semantics are otherwise unchanged: a recorder failure stays NON-FATAL to the
// webhook and still goes through reportArbitrageFailure so it is logged and
// counted (#423).
func (d *Dispatcher) recordArbitrage(ctx context.Context, sub subscription.StoreSubscription, in arbitrage.RecordInput) {
	// runCtx is the context the collector hands us at drain time (request
	// cancellation stripped, own timeout applied) — never the captured one.
	record := func(runCtx context.Context) error {
		if recErr := d.recorder.RecordIfFlagged(runCtx, in); recErr != nil {
			// Swallowed on purpose: the arbitrage write must not block the
			// subscription lifecycle or trigger a Stripe redelivery that
			// re-fires every other side effect. Silence is what was wrong
			// before (#423), so it is logged and counted instead.
			reportArbitrageFailure(sub, recErr)
		}
		return nil
	}

	if postcommit.Add(ctx, record) {
		return
	}

	// No collector in ctx — a caller that did not opt in (tests, or a future
	// entry point that forgot postcommit.WithDeferredSends). Run inline
	// rather than dropping the fraud signal, but say so loudly: a call site
	// that silently reverts to the inline path is exactly how the stall
	// above comes back, and it looks perfectly healthy in the logs while
	// doing it.
	slog.Default().Warn("dispatch: no post-commit collector in context; recording arbitrage INLINE, inside the webhook transaction — the caller of Dispatch is missing postcommit.WithDeferredSends (#438)",
		"store_id", sub.StoreID.String(),
		"tenant_id", sub.TenantID.String(),
		"subscription_id", sub.ID.String())

	_ = record(ctx)
}

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
		d.recordArbitrage(ctx, sub, arbitrage.RecordInput{
			SubscriptionID: sub.ID,
			TenantID:       sub.TenantID,
			StoreID:        sub.StoreID,
			PriceTier:      sub.PriceTier,
			CardCountry:    obj.PaymentMethodDetails.Card.Country,
			BillingCountry: obj.CustomerDetails.Address.Country,
			IPCountry:      "", // unknown at webhook time — evaluated as "??"
			RawIP:          "", // no raw IP at webhook time
		})
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

// handleSubscriptionUpdated refreshes period boundaries and cancel_at_period_end,
// and emits an audit event when one of those values actually TRANSITIONS.
//
// Why it reads the row first (#705). The UPDATE below is blind: it writes the
// payload's values without looking at what was there. Stripe emits
// customer.subscription.updated for many reasons, most of which change none of
// the three columns we mirror, so emitting on every delivery would produce one
// event per webhook rather than one per transition — noise that makes the
// audit trail worse. The pre-state is therefore read inside the SAME
// transaction, immediately before the UPDATE, and compared afterwards; see
// decidePeriodTransitions for the rule.
//
// It is a method (not the free function it used to be) purely so it can reach
// d.emitter — that structural fact is why the original TODO(P3) sat here
// unimplemented.
func (d *Dispatcher) handleSubscriptionUpdated(ctx context.Context, tx *gorm.DB, raw []byte) error {
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
	// Pre-state, inside the same locked transaction as the UPDATE below. A
	// missing row is NOT an error here: today's behaviour is that the UPDATE
	// touches zero rows and the handler returns nil (test-mode noise, a
	// customer we never provisioned), and that is preserved — we simply have
	// no before-state to compare and emit nothing.
	var before subscriptionPeriodState
	var pc periodTransitionContext
	haveBefore := false
	var prior subscription.StoreSubscription
	switch err := tx.WithContext(ctx).Where("stripe_customer_id = ?", obj.Customer).First(&prior).Error; {
	case err == nil:
		haveBefore = true
		before = subscriptionPeriodState{
			PeriodStart:       prior.CurrentPeriodStart,
			CancelAtPeriodEnd: prior.CancelAtPeriodEnd,
		}
		pc = periodTransitionContext{
			Customer:       obj.Customer,
			SubscriptionID: prior.ID,
			TenantID:       prior.TenantID,
			StoreID:        prior.StoreID,
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// Fall through to the UPDATE, which will affect zero rows.
	default:
		return fmt.Errorf("dispatch: subscription.updated lookup: %w", err)
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

	if haveBefore {
		d.emitPeriodTransitions(pc, before, subscriptionPeriodState{
			PeriodStart:       periodStart,
			CancelAtPeriodEnd: obj.CancelAtPeriodEnd,
		})
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
// The confirmation is suppressed when plan is still 'trial' — see
// sendTrialBilled — because the template would then name "trial" as the plan
// the merchant is being billed for.
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
// Idempotency guarantee, and why it is NOT first_charge_at: invoice.paid is
// dispatched as a chain (see dispatcher.go) whose later handler can fail — or
// the commit itself can fail — rolling first_charge_at back to NULL, and
// first_charge_at can also be reset later by a compensating correction or a
// restore. Stripe then redelivers and wasFirstCharge is true all over again.
// So the at-most-once guarantee comes from a billing_email_sends claim keyed
// on the constant period trialBilledPeriodKey — one per subscription for the
// life of the subscription, not one per event.
//
// Both the claim and the send happen at drain time, after the webhook
// transaction has committed. Do not move the claim back inside the
// transaction: the reason is spelled out at the send closure below, and
// getting it wrong silently costs the merchant their confirmation forever.
//
// Without d.db there is no claim store, and no way to make this at-most-once,
// so the send is skipped rather than risked.
func (d *Dispatcher) sendTrialBilled(ctx context.Context, tx *gorm.DB, sub subscription.StoreSubscription) {
	log := slog.Default()

	// The template says "your <plan> plan is active ... billed monthly". A
	// subscription is bootstrapped on plan='trial' and only advanced by
	// CommitUpgrade / CommitDowngrade / planchange's initial_selection, so a
	// merchant who reached first charge through the legacy CreateCheckoutSession
	// route (which never persists the plan) would be told their "trial plan" is
	// now being billed. That is a false billing statement, so skip the send.
	//
	// Ordering: this guard sits before the claim, which now lives in the send
	// closure below. That is the same rule as before — a skip sends nothing,
	// so there is nothing to deduplicate, and burning the
	// one-per-subscription slot here would permanently suppress a correct
	// confirmation if the plan is resolved before a later event lands.
	if sub.Plan == subscription.PlanTrial {
		log.Warn("dispatch: trial-billed email skipped — plan still 'trial' at first charge; no plan was ever recorded for this subscription",
			"store_id", sub.StoreID.String(),
			"tenant_id", sub.TenantID.String(),
			"reason", "plan_unresolved")
		d.countSkip("plan_unresolved")
		return
	}

	if d.db == nil {
		log.Warn("dispatch: trial-billed email skipped — no claim store wired (WithDB)",
			"store_id", sub.StoreID.String())
		d.countSkip("no_claim_store")
		return
	}

	to := ""
	if sub.Email != nil {
		to = *sub.Email
	}
	// Everything the send needs is resolved HERE, inside the transaction:
	// tx is dead once it commits, so the store name cannot be looked up from
	// the deferred closure.
	data := map[string]any{
		"store_id":   sub.StoreID.String(),
		"tenant_id":  sub.TenantID.String(),
		"store_name": subscription.StoreNameFor(ctx, tx, sub.StoreID),
		"plan":       string(sub.Plan),
		"period":     string(sub.SubscriptionPeriod),
	}

	// send claims the one-per-subscription slot and then makes the provider
	// HTTP call. Both run at drain time, after the transaction has committed.
	//
	// It takes its context as a parameter rather than capturing the request's:
	// postcommit.Run hands it a detached, timeout-bounded context so that a
	// request cancelled between the commit and the drain cannot fail the claim
	// and the send. The inline fallback below passes the request context, where
	// cancellation still should abort — nothing has committed yet there.
	//
	// Why the claim is HERE and not inside the transaction like the other
	// billing mail paths: the send is deferred, and a rolled-back transaction
	// is never drained — so on a rollback nothing is sent. A claim taken
	// inside the transaction would be written on d.db, survive that rollback,
	// and burn the slot for an email that never left. Stripe's retry would
	// then find the slot taken and send nothing, and since the retry does
	// commit first_charge_at, the template never fires again: the merchant
	// would silently never receive their billing confirmation. Claiming here
	// keeps the pair atomic in the only sense that matters — either both the
	// claim and the send happen, or neither does.
	//
	// What still prevents a duplicate: period_key is the constant
	// "first_charge" (see trialBilledPeriodKey), so the claim is
	// one-per-subscription for the life of the subscription, not per event.
	// If first_charge_at is later reset — a compensating correction, a
	// restore — and Stripe redelivers, wasFirstCharge is true again but the
	// claim row from the delivered send is still there and this send is
	// suppressed.
	//
	// d.db is deliberately still the non-transactional handle: at drain time
	// the webhook transaction is gone, so it is the only handle there is.
	send := func(ctx context.Context) error {
		won, claimErr := subscription.ClaimEmailSend(ctx, d.db, sub.ID,
			string(email.TemplateTrialStartedBilled), trialBilledPeriodKey, time.Now().UTC())
		if claimErr != nil {
			d.countSkip("claim_failed")
			return fmt.Errorf("dispatch: trial-billed claim failed for store %s; not sending: %w",
				sub.StoreID, claimErr)
		}
		if !won {
			// Already sent for this subscription — a redelivery after a
			// compensating reset, or a second pod. Nothing to do.
			return nil
		}

		if sendErr := d.emailCl.Send(ctx, email.TemplateTrialStartedBilled, to, data); sendErr != nil {
			d.countSkip(email.SkipReason(sendErr))
			// An address failure releases the claim; a transport failure keeps
			// it. Same split as the other four billing mail paths.
			//
			// Releasing matters more here than anywhere else: period_key is
			// the constant "first_charge", so the claim is one-per-subscription
			// for LIFE. A merchant still on a billing+<uuid>@mark8ly.local
			// placeholder at first charge would otherwise burn that slot
			// before any network call, and — because first_charge_at is
			// committed, so wasFirstCharge is false forever after — never
			// receive a trial-billed confirmation at all, however many real
			// addresses arrive later.
			if errors.Is(sendErr, email.ErrUndeliverable) {
				if relErr := subscription.ReleaseEmailClaim(ctx, d.db, sub.ID,
					string(email.TemplateTrialStartedBilled), trialBilledPeriodKey); relErr != nil {
					log.Error("dispatch: trial-billed release claim failed",
						"store_id", sub.StoreID.String(), "err", relErr.Error())
				} else {
					log.Info("dispatch: trial-billed claim released for retry",
						"store_id", sub.StoreID.String())
				}
			}
			return fmt.Errorf("dispatch: trial-billed email not sent for store %s (reason %s): %w",
				sub.StoreID, email.SkipReason(sendErr), sendErr)
		}
		if d.sent != nil {
			d.sent.WithTemplate(string(email.TemplateTrialStartedBilled)).Inc()
		}
		return nil
	}

	// Hand the send to the request's collector so the caller runs it after
	// the advisory-lock transaction commits — a SendGrid call (15s timeout,
	// plus a possible Resend fallback) must not hold the per-store lock and
	// a pool connection against Stripe's 30s webhook budget.
	//
	// Note the claim is NOT taken here — it is the first statement of the
	// send closure, and travels with the send. The rationale is above; the
	// short version is that a claim written here would survive a rollback
	// that discarded the send it paid for.
	if postcommit.Add(ctx, send) {
		return
	}

	// No collector in ctx — a caller that did not opt in (tests, or a future
	// entry point that forgot to install one). Send inline rather than
	// dropping it: the old behaviour is slow, but silently losing a
	// merchant's billing email is worse.
	//
	// Entering this path is itself worth a warning. Without it a new call
	// site that forgets postcommit.WithDeferredSends silently reverts to
	// making the provider call under the advisory lock — the exact behaviour
	// this indirection exists to remove — and looks perfectly healthy in the
	// logs while doing it.
	log.Warn("dispatch: no post-commit collector in context; sending trial-billed email INLINE, inside the webhook transaction — the caller of Dispatch is missing postcommit.WithDeferredSends",
		"store_id", sub.StoreID.String(),
		"tenant_id", sub.TenantID.String())

	if sendErr := send(ctx); sendErr != nil {
		// Don't fail the webhook — Stripe would retry, double-firing every
		// other side effect. Email failure is a soft error: log and move on.
		log.Warn("dispatch: trial-billed email not sent",
			"store_id", sub.StoreID.String(),
			"reason", email.SkipReason(sendErr),
			"err", sendErr.Error())
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
	//
	// Deliberately NOT stamping updated_at here: it is not a business-state
	// change, it is bookkeeping. store_subscriptions.updated_at is both the
	// win-back cron's 30-to-31-day eligibility window AND its idempotency
	// key, and it also feeds the expired->store_closed timer and the 150-day
	// hard-delete cutoff (see cmd/backfill-email for the original fix of
	// this exact bug). Stripe keeps retrying failed invoices for weeks after
	// a subscription is already expired, so this handler runs against
	// already-terminal rows far more often than the SCA path below — bumping
	// updated_at here would silently move or drop those merchants from the
	// win-back window and push back their retention timers.
	if e.Data.Object.HostedInvoiceURL != "" {
		res := tx.WithContext(ctx).Exec(`
			UPDATE store_subscriptions
			SET hosted_invoice_url = ?
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
	//
	// Deliberately NOT stamping updated_at here either — same reasoning as
	// handleInvoicePaymentFailed above: this UPDATE only touches the
	// bookkeeping hosted_invoice_url column, not status, so it must not move
	// the win-back cron's eligibility/idempotency window or the lifecycle
	// crons' retention timers.
	if e.Data.Object.HostedInvoiceURL != "" {
		res := tx.WithContext(ctx).Exec(`
			UPDATE store_subscriptions
			SET hosted_invoice_url = ?
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
