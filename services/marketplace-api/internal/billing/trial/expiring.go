package trial

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// DefaultExpiryWindow is how far ahead the console looks for expiring trials
// when a caller supplies no `days`. Shared by GET /admin/kpis's
// trials_expiring counter and GET /admin/billing/trials, so the two cannot
// report different numbers for the same word.
const DefaultExpiryWindow = 7 * 24 * time.Hour

// MaxExpiryWindow clamps `days`. An operator-extended trial (trial_ends_at
// set via the console) can end arbitrarily far in the future, so this is
// NOT a claim that every live trial ends within TrialDays — it is only a
// bound to keep the window finite.
const MaxExpiryWindow = 365 * 24 * time.Hour

// ExpiringRow is one trial about to expire.
type ExpiringRow struct {
	TenantID         string
	StoreID          string
	TrialEndsAt      time.Time
	Plan             string
	Period           string
	BillingCurrency  *string
	PriceTier        subscription.PriceTier
	HasPaymentMethod bool
	Status           string
}

// expiringScope narrows to trials that will actually EXPIRE, in the window
// (asOf, asOf+window].
//
// All three clauses matter, and the third is the one #282 originally missed:
//
//   - status = 'trialing'
//   - stripe_subscription_id IS NULL — no card. A trialing subscription WITH
//     a card has a Stripe subscription and will CONVERT, not expire; its
//     renewal date comes from Stripe, not from us.
//   - effective trial end inside the window. This is the same rule
//     expiry_cron.go applies and the same date the merchant is shown.
//
// The window's brackets and the index-preserving two-branch predicate both
// live in EndsBetweenScope — see endsat.go.
func expiringScope(db *gorm.DB, asOf time.Time, window time.Duration) *gorm.DB {
	return EndsBetweenScope(
		db.Model(&subscription.StoreSubscription{}).
			Where("status = ?", subscription.StatusTrialing).
			Where("stripe_subscription_id IS NULL"),
		asOf, asOf.Add(window),
	)
}

// CountExpiring counts trials that will expire in the window (asOf,
// asOf+window], per expiringScope's rule. asOf and window are parameters
// (never time.Now() internally) so callers — and tests — can pin the
// boundary exactly; production passes time.Now().
func CountExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration) (int64, error) {
	var n int64
	err := expiringScope(db.WithContext(ctx), asOf, window).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("trial: count expiring: %w", err)
	}
	return n, nil
}

// ListExpiring returns a page of trials expiring in the window (asOf,
// asOf+window], ordered by soonest effective end first, along with the
// unpaginated total match count.
func ListExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int) ([]ExpiringRow, int64, error) {
	total, err := CountExpiring(ctx, db, asOf, window)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit

	var raw []subscription.StoreSubscription
	// Soonest effective end first. This used to order by created_at on the
	// assumption that every row shared one trial length — extensions break
	// that, so an older row extended further out must sort after a newer one
	// ending sooner.
	// Built in hours, not days: INTERVAL '<n> days' is calendar arithmetic
	// evaluated in the session timezone, which can disagree with EndsAt's
	// exact 90*24h across a DST boundary in a non-UTC session. This is
	// ordering-only, but it must still agree with EndsAt or rows sort
	// inconsistently with what they're compared against.
	err = expiringScope(db.WithContext(ctx), asOf, window).
		Order("COALESCE(trial_ends_at, created_at + INTERVAL '" + strconv.Itoa(TrialDays*24) + " hours') ASC").
		Offset(offset).
		Limit(limit).
		Find(&raw).Error
	if err != nil {
		return nil, 0, fmt.Errorf("trial: list expiring: %w", err)
	}

	rows := make([]ExpiringRow, 0, limit)
	for _, r := range raw {
		rows = append(rows, ExpiringRow{
			TenantID:         r.TenantID.String(),
			StoreID:          r.StoreID.String(),
			TrialEndsAt:      EndsAt(r),
			Plan:             string(r.Plan),
			Period:           string(r.SubscriptionPeriod),
			BillingCurrency:  r.BillingCurrency,
			PriceTier:        r.PriceTier,
			HasPaymentMethod: r.HasDefaultPaymentMethod,
			Status:           string(r.Status),
		})
	}
	return rows, total, nil
}
