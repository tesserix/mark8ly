package trial

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
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
	// ErrStripeManaged: as of #358, this does NOT mean "this trial is
	// Stripe-managed, refuse it" — card-backed trials are extended through
	// Stripe below. It means the trial IS card-backed but this build has no
	// Stripe client configured (e.Stripe == nil), so there is nowhere to
	// send the update and a local-only write would put the console and
	// Stripe in disagreement about when a real merchant is charged. The
	// wire code stays "stripe_managed" — it is a live contract shipped by
	// #286 and the caller-visible meaning, "we will not move this trial's
	// date here", is unchanged.
	ErrStripeManaged  = errors.New("trial: trial is stripe-managed")
	ErrNotTrialing    = errors.New("trial: subscription is not in a trial state")
	ErrEndNotInFuture = errors.New("trial: new trial end must be in the future")

	// ErrStripeStateConflict: the local row says trialing but Stripe does
	// not. Reconciling silently is not this endpoint's job.
	ErrStripeStateConflict = errors.New("trial: stripe subscription is not trialing")
	// ErrTrialEndTooFar: Stripe bounds trial_end at two years FROM THE
	// CURRENT billing_cycle_anchor — not from now, which is a different
	// instant whenever the anchor is not near now.
	ErrTrialEndTooFar = errors.New("trial: new trial end is more than two years from the stripe billing anchor")
	// ErrTrialEndNotAfterStripe: the requested end is not strictly AFTER the
	// trial_end Stripe currently holds, so applying it would move a real
	// merchant's billing date EARLIER — the direction #358 rejects.
	//
	// The comparison is deliberately against STRIPE's value and never
	// against the local derived end: for a never-extended card-backed row
	// trial_ends_at is NULL, so EndsAt(sub) reports created_at+90d, which is
	// exactly the stale number that made this reachable. Comparing against
	// the stale value would refuse every informed retry too and leave the
	// operator permanently stuck.
	//
	// NARROWING, STATED OUTRIGHT: this makes SHORTENING a card-backed trial
	// impossible through this endpoint. That is deliberate — this route is
	// "extend", and an accidental shortening bills a merchant early, which
	// is not recoverable by a later correction. The card-LESS path is
	// unchanged: shortening a card-less trial stays legal (#358).
	ErrTrialEndNotAfterStripe = errors.New("trial: new trial end is not after the trial end stripe currently holds")
	// ErrStripeCall: Stripe was reached for and did not succeed. Nothing was
	// written locally; the caller may retry.
	ErrStripeCall = errors.New("trial: stripe call failed")
	// ErrStripeAppliedLocalWriteFailed: the ONE divergence this design
	// accepts has actually happened. Stripe moved the merchant's billing
	// anchor and the local write then failed, so the console still shows the
	// old date while the merchant is really billed on the new one. This is
	// NOT a routine database error and must never be reported as one: it
	// needs a human to reconcile, so the wrapped message carries the Stripe
	// subscription id and the exact trial_end Stripe now holds (#358).
	ErrStripeAppliedLocalWriteFailed = errors.New("trial: stripe trial end was moved but the local write failed")
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

// NewExtender constructs an Extender.
//
// su may be nil, and a TYPED nil — a nil *stripe.Client assigned into the
// interface — is normalised to a true nil here rather than being left to
// panic at first use. Both mean the same thing to a reader ("no Stripe
// configured"), but only one of them means it to Go: an interface holding a
// nil pointer is itself non-nil, so `e.Stripe != nil` would be true and the
// first method call would panic INSIDE the transaction, after the row lock
// was taken — with the operator seeing a 500 for a request that changed
// nothing, or worse, in a variant of this shape, for one that changed
// everything (#288).
//
// Enforced here rather than documented at the call site because a call site
// can be copied wrongly and a comment cannot fail a test.
func NewExtender(su StripeTrialUpdater) *Extender {
	if su != nil {
		if v := reflect.ValueOf(su); v.Kind() == reflect.Ptr && v.IsNil() {
			su = nil
		}
	}
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
//
// callerIdemKey is the caller's own already-scoped idempotency key (the
// handler's "trial_extend:<store_id>:<Idempotency-Key header>"). It is not
// used for any local write — the handler owns idempotency_keys — it exists
// only so the key sent to Stripe can be derived from the REQUEST as well as
// from the target date. See stripeIdempotencyKey (#358).
func (e *Extender) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time, callerIdemKey string) (ExtendResult, error) {
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
				return fmt.Errorf("%w: get subscription: %w", ErrStripeCall, err)
			}
			if current.Status != "trialing" {
				return ErrStripeStateConflict
			}

			// An "extend" must never move a card-backed trial EARLIER.
			// Stripe's trial_end is the only value that says when the
			// merchant is actually charged; our local derived end can be
			// older than it (trial_ends_at is NULL until the first
			// extension, so the console shows created_at+90d). Without this
			// an operator picking a date they believe is later than what the
			// console shows can pull a real billing date forward, and we
			// would report success with billing_anchor_moved: true (#358).
			//
			// Both dates go in the message so the operator can retry
			// immediately with an informed one instead of guessing.
			if end.Unix() <= current.TrialEnd {
				return fmt.Errorf("%w: requested %s, stripe currently holds %s",
					ErrTrialEndNotAfterStripe,
					end.Format(time.RFC3339),
					time.Unix(current.TrialEnd, 0).UTC().Format(time.RFC3339))
			}

			anchor := time.Unix(current.BillingCycleAnchor, 0).UTC()
			if end.After(anchor.Add(maxStripeTrialWindow)) {
				return ErrTrialEndTooFar
			}

			updated, err := e.Stripe.UpdateTrialEnd(sctx, billingstripe.UpdateTrialEndParams{
				SubscriptionID: stripeID,
				TrialEnd:       end.Unix(),
				// Derived from the CALLER's key and the absolute target
				// second — see stripeIdempotencyKey for why both halves are
				// load-bearing (#358 F1).
				IdempotencyKey: stripeIdempotencyKey(callerIdemKey, sub.StoreID, end),
				Metadata: map[string]string{
					"mark8ly_store_id":  sub.StoreID.String(),
					"mark8ly_tenant_id": sub.TenantID.String(),
				},
			})
			if err != nil {
				return fmt.Errorf("%w: update trial end: %w", ErrStripeCall, err)
			}

			// A (nil, nil) return would panic on the deref below — inside an
			// open transaction still holding the FOR UPDATE row lock, which
			// is a far worse failure than an error. The real client never
			// does this; the guard is about the failure mode, not the odds.
			if updated == nil {
				return fmt.Errorf("%w: update trial end: stripe returned no subscription", ErrStripeCall)
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
		// The closure wrote into `out` before the rollback, so `out` still
		// records whether Stripe was actually moved. A rolled-back
		// transaction undoes our row; it cannot undo Stripe.
		if out.StripeApplied {
			return ExtendResult{}, fmt.Errorf(
				"%w: stripe subscription %s now holds trial_end=%d (previously trial_end=%d, billing_cycle_anchor=%d): %w",
				ErrStripeAppliedLocalWriteFailed, out.StripeSubscriptionID, out.StripeTrialEnd,
				out.PreviousStripeTrialEnd, out.PreviousBillingAnchor, err)
		}
		return ExtendResult{}, err
	}
	return out, nil
}

// stripeIdempotencyKey derives the key handed to Stripe's UpdateTrialEnd.
//
// It combines TWO things, and both are load-bearing (#358 F1):
//
//   - the caller's already-scoped key, which is unique per operator REQUEST
//     ("trial_extend:<store_id>:<Idempotency-Key header>"), and
//   - the absolute target second.
//
// The caller's key is what makes two DIFFERENT operator requests distinct
// even when they name the same date. Deriving the key from the store and
// the date alone made this sequence, all inside Stripe's 24h idempotency
// window, a silent no-op: extend to Dec 1 (key A, applied), correct to
// Nov 1 (key B, applied), then change BACK to Dec 1 — key A again, same
// subscription, same params, so Stripe replays its CACHED Dec 1 response
// and changes nothing. The reply then reports Dec 1, so no check on our
// side can notice; the console would say Dec 1 while Stripe billed Nov 1,
// charging the merchant EARLIER than shown.
//
// The target second is what keeps a caller who reuses one header key across
// two different bodies from getting the first body's outcome.
//
// A GENUINE retry — the same operator resending with the SAME
// Idempotency-Key header and the same date — still produces the SAME key,
// so Stripe still dedupes it. That is deliberate: it is what lets a retry
// after a local-write failure converge instead of extending twice.
//
// The empty-key fallback preserves the pre-#358 format for non-HTTP callers
// (the handler makes the header mandatory, so production always supplies
// one); it is weaker, which is exactly why the route requires a key.
func stripeIdempotencyKey(callerIdemKey string, storeID uuid.UUID, end time.Time) string {
	base := strings.TrimSpace(callerIdemKey)
	if base == "" {
		base = "trial_extend:" + storeID.String()
	}
	return base + ":" + strconv.FormatInt(end.Unix(), 10)
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
