package trial

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// DefaultExpiryWindow is how far ahead the console looks for expiring trials
// when a caller supplies no `days`. Shared by GET /admin/kpis's
// trials_expiring counter and GET /admin/billing/trials, so the two cannot
// report different numbers for the same word.
const DefaultExpiryWindow = 7 * 24 * time.Hour

// MaxExpiryWindow clamps `days`. Beyond the trial length the window stops
// meaning anything — every live trial is inside it.
const MaxExpiryWindow = time.Duration(TrialDays) * 24 * time.Hour

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
//     renewal date comes from Stripe, not from created_at.
//   - created_at + TrialDays inside the window. This is the same rule
//     expiry_cron.go applies and the same date the merchant is shown.
//
// Half-open left so an already-expired trial is not "expiring"; inclusive
// right so one ending exactly at the edge is.
//
// Note the algebra: created_at + TrialDays > asOf is created_at > asOf -
// TrialDays. Doing it this way keeps the comparison on a plain indexed
// column instead of an expression — do not "simplify" it back to comparing
// against an expression on created_at.
func expiringScope(db *gorm.DB, asOf time.Time, window time.Duration) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	return db.Model(&subscription.StoreSubscription{}).
		Where("status = ?", subscription.StatusTrialing).
		Where("stripe_subscription_id IS NULL").
		Where("created_at > ?", asOf.Add(-trialLen)).
		Where("created_at <= ?", asOf.Add(window).Add(-trialLen))
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
// asOf+window], ordered by created_at ASC (soonest trial end first, since
// every row shares the same trial length), along with the unpaginated total
// match count.
func ListExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int) ([]ExpiringRow, int64, error) {
	total, err := CountExpiring(ctx, db, asOf, window)
	if err != nil {
		return nil, 0, err
	}

	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	offset := (page - 1) * limit

	var raw []subscription.StoreSubscription
	err = expiringScope(db.WithContext(ctx), asOf, window).
		Order("created_at ASC").
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
			TrialEndsAt:      r.CreatedAt.Add(trialLen),
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
