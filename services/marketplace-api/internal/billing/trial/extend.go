package trial

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Errors returned by Extend. Each maps to a distinct HTTP code at the
// handler, so the console can tell "already converted" from "expired" from
// "Stripe owns this one" rather than getting one opaque refusal.
var (
	ErrNoSubscription   = errors.New("trial: no subscription for store")
	ErrAlreadyConverted = errors.New("trial: subscription already converted")
	ErrStripeManaged    = errors.New("trial: trial is stripe-managed")
	ErrNotTrialing      = errors.New("trial: subscription is not in a trial state")
	ErrEndNotInFuture   = errors.New("trial: new trial end must be in the future")

	// ErrStripeStateConflict: the local row says trialing but Stripe does
	// not. Reconciling silently is not this endpoint's job.
	ErrStripeStateConflict = errors.New("trial: stripe subscription is not trialing")
	// ErrTrialEndTooFar: Stripe bounds trial_end at two years FROM THE
	// CURRENT billing_cycle_anchor — not from now, which is a different
	// instant whenever the anchor is not near now.
	ErrTrialEndTooFar = errors.New("trial: new trial end is more than two years from the stripe billing anchor")
	// ErrStripeCall: Stripe was reached for and did not succeed. Nothing was
	// written locally; the caller may retry.
	ErrStripeCall = errors.New("trial: stripe call failed")
)

// maxStripeTrialWindow mirrors Stripe's documented bound on
// SubscriptionUpdateParams.TrialEnd: "Can be at most two years from
// billing_cycle_anchor". Validated locally so the operator gets our error
// envelope and the actual bound, rather than an opaque Stripe 400.
const maxStripeTrialWindow = 2 * 365 * 24 * time.Hour

// stripeCallTimeout bounds how long the store_subscriptions row lock is held
// across the external call. The lock is deliberate — it is what removes the
// window in which the row could convert while Stripe is in flight — and this
// ceiling is what keeps "deliberate" from becoming "indefinite".
const stripeCallTimeout = 10 * time.Second

// ExtendResult describes a completed extension.
type ExtendResult struct {
	SubscriptionID   uuid.UUID
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	PreviousEndsAt   time.Time
	NewEndsAt        time.Time
	RemindersCleared int64

	// StripeApplied is true only when this extension moved the trial end in
	// Stripe. False for every card-less extension. The handler surfaces it so
	// an operator learns from the same call whether a billing anchor moved.
	StripeApplied bool

	// The Stripe-side facts, populated only when StripeApplied is true.
	// StripeTrialEnd is read from Stripe's REPLY, never echoed from our
	// request: what we asked for and what Stripe stored are two claims.
	StripeSubscriptionID   string
	StripeTrialEnd         int64
	PreviousStripeTrialEnd int64
	PreviousBillingAnchor  int64
}

// StripeTrialUpdater is the subset of the Stripe client this package needs,
// declared here rather than imported as a concrete type so the extension can
// be tested without a live Stripe and so the dependency points inward.
type StripeTrialUpdater interface {
	GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error)
	UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error)
}

// Extender owns "move a trial's end date", for both card-less and
// card-backed trials.
//
// A nil Stripe field is a SUPPORTED configuration, not a degraded one: a
// build without STRIPE_BILLING_SECRET_KEY refuses card-backed trials with
// ErrStripeManaged, exactly as this endpoint did before #358. Callers MUST
// leave the interface nil rather than assigning a nil *stripe.Client into
// it — a typed nil makes `e.Stripe != nil` true and panics on first use.
type Extender struct {
	Stripe StripeTrialUpdater
}

// NewExtender constructs an Extender. su may be nil; see the type's comment.
func NewExtender(su StripeTrialUpdater) *Extender {
	return &Extender{Stripe: su}
}

// Extend moves a trial's end date, refusing the states where doing so
// would be wrong or would disagree with Stripe.
//
// Everything happens in one transaction, and the row is re-read INSIDE it,
// so the refusal checks and the write see the same state — otherwise a
// subscription that converts between the check and the write would be
// extended anyway.
//
// now is a parameter rather than time.Now() so callers and tests can pin
// the boundary exactly; production passes time.Now().UTC().
func (e *Extender) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (ExtendResult, error) {
	var out ExtendResult

	// Checked before opening a transaction: it needs no row, and refusing
	// early keeps a pointless BEGIN off the connection pool.
	if !newEnd.After(now) {
		return out, ErrEndNotInFuture
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub subscription.StoreSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("store_id = ?", storeID).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoSubscription
			}
			return fmt.Errorf("trial: load subscription: %w", err)
		}

		// Order matters: `active` gets its own error even though it would
		// also fail the trial-state check, because "already converted" is
		// the acceptance criterion's own words and the console shows a
		// different message for it.
		switch {
		case sub.Status == subscription.StatusActive:
			return ErrAlreadyConverted
		case sub.Status != subscription.StatusTrialing && sub.Status != subscription.StatusSignup:
			return ErrNotTrialing
		}

		// A trial whose EFFECTIVE end has already passed but whose status
		// is still `trialing` — the window between the end passing and the
		// 00:15 expiry cron sweeping it to `not_trialing` — must refuse the
		// same way the post-cron state does. Using the SAME sentinel,
		// ErrNotTrialing, is the point: the operator's answer must not
		// depend on whether the cron happened to run yet. Reinstating an
		// already-expired trial is out of scope (see the spec).
		if !EndsAt(sub).After(now) {
			return ErrNotTrialing
		}

		// The EFFECTIVE end before the write — the derived date when the
		// trial has never been extended. Never recompute it here; EndsAt is
		// the only definition (#353).
		out.PreviousEndsAt = EndsAt(sub)

		end := newEnd.UTC()

		// Card-backed: Stripe owns the billing date, so Stripe moves FIRST
		// and is the source of truth. The row lock taken above is held
		// across this call, so nothing can convert or re-extend underneath
		// it; stripeCallTimeout bounds how long that lock can live.
		//
		// The ordering is the decision #358 required be made deliberately:
		// if this call fails the transaction rolls back and NOTHING is
		// written locally. If instead the commit below fails, Stripe is
		// AHEAD of us — the merchant is billed LATER than the console shows,
		// which is the safe direction. The reverse ordering fails the other
		// way.
		if stripeID := stripeSubscriptionID(sub); stripeID != "" {
			if e.Stripe == nil {
				// No Stripe configured: refuse exactly as this endpoint did
				// before #358. A local-only extension of a Stripe-managed
				// trial would put the console and Stripe in disagreement
				// about when a real merchant is charged.
				return ErrStripeManaged
			}

			sctx, cancel := context.WithTimeout(ctx, stripeCallTimeout)
			defer cancel()

			current, err := e.Stripe.GetSubscription(sctx, stripeID)
			if err != nil {
				return fmt.Errorf("%w: get subscription: %v", ErrStripeCall, err)
			}
			if current.Status != "trialing" {
				return ErrStripeStateConflict
			}
			anchor := time.Unix(current.BillingCycleAnchor, 0).UTC()
			if end.After(anchor.Add(maxStripeTrialWindow)) {
				return ErrTrialEndTooFar
			}

			updated, err := e.Stripe.UpdateTrialEnd(sctx, billingstripe.UpdateTrialEndParams{
				SubscriptionID: stripeID,
				TrialEnd:       end.Unix(),
				// Derived from the store, so a retry of the SAME extension
				// cannot move the date twice, while a different extension of
				// the same store still can.
				IdempotencyKey: "trial_extend:" + sub.StoreID.String() + ":" + strconv.FormatInt(end.Unix(), 10),
				Metadata: map[string]string{
					"mark8ly_store_id":  sub.StoreID.String(),
					"mark8ly_tenant_id": sub.TenantID.String(),
				},
			})
			if err != nil {
				return fmt.Errorf("%w: update trial end: %v", ErrStripeCall, err)
			}

			out.StripeApplied = true
			out.StripeSubscriptionID = stripeID
			out.StripeTrialEnd = updated.TrialEnd
			out.PreviousStripeTrialEnd = current.TrialEnd
			out.PreviousBillingAnchor = current.BillingCycleAnchor
		}

		if err := tx.Model(&subscription.StoreSubscription{}).
			Where("store_id = ?", storeID).
			Update("trial_ends_at", end).Error; err != nil {
			return fmt.Errorf("trial: write trial_ends_at: %w", err)
		}

		// Clear the reminder slots so the cadence re-arms against the new
		// end. trial_reminders' PK is (subscription_id, offset_key) and
		// processOne inserts ON CONFLICT DO NOTHING, so a reminder already
		// sent can NEVER re-send: without this, a merchant extended past
		// their T-15 warning gets no notice before the date they are
		// actually charged on.
		res := tx.Exec(`DELETE FROM trial_reminders WHERE subscription_id = ?`, sub.ID)
		if res.Error != nil {
			return fmt.Errorf("trial: clear reminders: %w", res.Error)
		}

		out.SubscriptionID = sub.ID
		out.TenantID = sub.TenantID
		out.StoreID = sub.StoreID
		out.NewEndsAt = end
		out.RemindersCleared = res.RowsAffected
		return nil
	})
	if err != nil {
		return ExtendResult{}, err
	}
	return out, nil
}

// stripeSubscriptionID returns the subscription's Stripe id, or "" when it
// has none. A nil pointer and a pointer to "" mean the same thing here — the
// trial is card-less — and collapsing them at one site keeps every caller
// from having to remember that.
func stripeSubscriptionID(sub subscription.StoreSubscription) string {
	if sub.StripeSubscriptionID == nil {
		return ""
	}
	return *sub.StripeSubscriptionID
}
