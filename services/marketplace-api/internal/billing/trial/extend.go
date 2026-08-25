package trial

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

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
)

// ExtendResult describes a completed extension.
type ExtendResult struct {
	SubscriptionID   uuid.UUID
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	PreviousEndsAt   time.Time
	NewEndsAt        time.Time
	RemindersCleared int64
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
func Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (ExtendResult, error) {
	var out ExtendResult

	// Checked before opening a transaction: it needs no row, and refusing
	// early keeps a pointless BEGIN off the connection pool.
	if !newEnd.After(now) {
		return out, ErrEndNotInFuture
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub subscription.StoreSubscription
		if err := tx.Where("store_id = ?", storeID).First(&sub).Error; err != nil {
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
		case sub.StripeSubscriptionID != nil && *sub.StripeSubscriptionID != "":
			return ErrStripeManaged
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
