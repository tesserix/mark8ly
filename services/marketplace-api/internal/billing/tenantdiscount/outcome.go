package tenantdiscount

import "github.com/google/uuid"

// Outcome is what happened to ONE store. Every store the tenant owns gets
// one, including the stores where nothing happened: the plan's rule is that a
// store with no subscription is an explicit outcome, never a silent skip, and
// an outcome set that only names successes cannot express that.
type Outcome string

const (
	// OutcomeApplied — the coupon was not on the subscription and this call
	// attached it. This is the only Apply outcome that changed Stripe.
	OutcomeApplied Outcome = "applied"

	// OutcomeAlreadyApplied — the coupon was already on the subscription, so
	// nothing was sent. This is idempotent APPLICATION, and it is still not
	// the "at most one active override per tenant" guarantee #660 asserts.
	// Migration 000132 added a local record of what THIS SERVICE applied, and
	// ErrOverrideAlreadyRecorded refuses a second recorded override — but
	// neither can see a coupon attached out of band, so neither can answer
	// what a Stripe customer is really carrying.
	OutcomeAlreadyApplied Outcome = "already_applied"

	// OutcomeRemoved — the coupon was on the subscription and this call took
	// it off. The only Remove outcome that changed Stripe.
	OutcomeRemoved Outcome = "removed"

	// OutcomeNotApplied — Remove found the coupon was not attached, so no
	// update was sent.
	OutcomeNotApplied Outcome = "not_applied"

	// OutcomePending — the store's subscription row exists but its
	// stripe_subscription_id is NULL, so there is no Stripe subscription to
	// carry a discount. That is the card-less trialing tenant, which is
	// exactly the population an operator discounts. For Remove there is
	// nothing attached to take off.
	//
	// "Pending" still names what did not happen in THIS call — it is not a
	// queue, and nothing retries it. What changed with migration 000132 is
	// that the store no longer has to be re-applied by hand: the SAME Apply
	// that reported this outcome first wrote the tenant's override to
	// tenant_applied_discounts, and the two places this service creates a
	// Stripe subscription (trial.Subscriber.subscribeInTx and
	// planchange.Orchestrator.executeInitialSubscription) read that record
	// and attach the coupon to the subscription they create. A store reported
	// pending is picked up when one of those two creates its subscription,
	// not before — and NOT when the subscription is created by hosted Stripe
	// Checkout, which this service only learns about from a webhook. See the
	// migration 000132 header.
	//
	// BE EXACT ABOUT WHAT IS DURABLE. What is durable is THIS SERVICE'S
	// RECORD OF WHAT IT APPLIED, and only for a tenant an Apply reached. What
	// is NOT:
	//
	//   - the grant itself, which lives in the console's
	//     `tenant_pricing_override_coupons` (tesserix-home 0047) and is still
	//     unreadable from here. A grant the console minted and never
	//     successfully sent to this service leaves nothing behind, and a
	//     store created afterwards gets no discount;
	//
	//   - the reverse: a console-side retirement that never reaches this
	//     service leaves our record live, and new stores keep receiving a
	//     coupon the console believes it withdrew;
	//
	//   - anything about a coupon attached out of band. Only what this
	//     service applied through Apply — or through the creation hook — is
	//     recorded.
	//
	// The application to the new subscription is also NON-FATAL by design: it
	// is never allowed to block a subscription being created, so it can fail
	// and be logged, leaving the store undiscounted with the tenant's record
	// still live.
	OutcomePending Outcome = "pending"

	// OutcomeNoOverride — the tenant holds no live recorded override, so
	// there was nothing to apply. Produced ONLY by ApplyToNewSubscription,
	// never by the Apply/Remove fan-out, which is always called with an
	// explicit coupon. It is the ordinary case on the subscription-creation
	// path: most tenants have no override at all.
	OutcomeNoOverride Outcome = "no_override"

	// OutcomeNoSubscription — the store has no store_subscriptions row at
	// all. Nothing to lock and nothing to bill; reported rather than skipped.
	OutcomeNoSubscription Outcome = "no_subscription"

	// OutcomeNoStripeCustomer — the subscription row exists but its
	// stripe_customer_id is empty, which is an incompletely provisioned row
	// rather than a normal state. Guarded like
	// internal/billing/appaddon/handler.go:133 guards it, and checked BEFORE
	// the pending check: a card-less trial does have a customer, so a row
	// missing one is an anomaly worth surfacing under its own name rather
	// than filing under "pending" where it would look routine.
	OutcomeNoStripeCustomer Outcome = "no_stripe_customer"

	// OutcomeFailed — this store's transaction rolled back. StoreResult.Err
	// and StoreResult.FailureCode say why.
	OutcomeFailed Outcome = "failed"
)

// FailureCode names the stage a failed store failed at, so a caller can build
// a distinct error code per stage without matching on driver text.
type FailureCode string

const (
	// FailureLoadSubscription — the locking read of store_subscriptions
	// failed for a reason other than "no row".
	FailureLoadSubscription FailureCode = "load_subscription_failed"

	// FailureStripeCall — Stripe was reached for and did not succeed.
	// Nothing was written for this store; the caller may retry.
	FailureStripeCall FailureCode = "stripe_call_failed"

	// FailureAuditWrite — the EmitTx insert failed. When Stripe had already
	// been changed, this is the divergence: see
	// ErrStripeChangedAuditWriteFailed.
	FailureAuditWrite FailureCode = "audit_write_failed"

	// FailureCommit — the closure succeeded and the COMMIT did not. Treated
	// as the same class as FailureAuditWrite when Stripe had been changed,
	// because the audit row is the only thing the transaction carried.
	FailureCommit FailureCode = "commit_failed"
)

// StoreResult is one store's line in the report.
type StoreResult struct {
	StoreID uuid.UUID

	// SubscriptionID is the local store_subscriptions row id, or uuid.Nil
	// when the store has none (OutcomeNoSubscription).
	SubscriptionID uuid.UUID

	// StripeCustomerID and StripeSubscriptionID are read from the local row.
	// StripeSubscriptionID is "" for a card-less trial (OutcomePending), and
	// both are "" when there is no row at all.
	StripeCustomerID     string
	StripeSubscriptionID string

	Outcome Outcome

	// Err and FailureCode are populated only when Outcome is OutcomeFailed.
	// Err carries the wrapped cause and the sentinel, if any, that names it.
	Err         error
	FailureCode FailureCode
}

// Changed reports whether this store's Stripe subscription was actually
// modified by the call. Used to decide whether a later failure is the
// unattributable-discount divergence or an ordinary one.
func (r StoreResult) Changed() bool {
	return r.Outcome == OutcomeApplied || r.Outcome == OutcomeRemoved
}

// Result is the whole fan-out: one line per store the tenant owns, in a
// deterministic order (stores.id ascending) so two runs of the same request
// read the same way.
type Result struct {
	TenantID uuid.UUID
	CouponID string
	Stores   []StoreResult
}
