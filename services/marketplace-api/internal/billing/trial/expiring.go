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
	// StripeManaged is true when the row has a Stripe subscription: it will
	// CONVERT rather than expire, and its renewal date comes from Stripe.
	// See ListOptions.IncludeStripeManaged.
	StripeManaged bool
}

// ListOptions varies what ListExpiring returns. The zero value is the
// contract #285 already ships, so an omitted option can never widen a live
// result set by accident.
type ListOptions struct {
	// IncludeStripeManaged adds trials that have a Stripe subscription.
	// They do not EXPIRE — they convert — so they are excluded by default
	// and from CountExpiring entirely. They are listable because #358 makes
	// them extendable, and an endpoint the console cannot discover a store
	// id for is unreachable in practice.
	IncludeStripeManaged bool

	// IncludeSignup adds trials whose status is still `signup`: a tenant on
	// a trial plan that never completed Stripe checkout. subscription
	// Service.Bootstrap creates every subscription that way, and the only
	// writer of signup -> trialing in this service is the
	// checkout.session.completed webhook handler (internal/billing/dispatch),
	// so a tenant that never checks out stays at `signup` indefinitely and
	// is invisible to this list under every window.
	//
	// Off by default, and — like IncludeStripeManaged — not reachable from
	// CountExpiring at all, because a `signup` row's trial end is NOTIONAL.
	// ExpiryCron selects status = 'trialing', so nothing will act on that
	// date: the row is not queued to expire, and this list showing it must
	// not be read as the expiry machinery watching it. It is listed so an
	// operator can chase the tenant to complete checkout, which is a
	// different action from an expiring trial's.
	IncludeSignup bool
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
	return trialingInWindowScope(db, asOf, window, false).Where("stripe_subscription_id IS NULL")
}

// trialingInWindowScope is expiringScope without the card filter: the status
// matches and the effective end lies in the window.
//
// includeSignup widens the status set from `trialing` alone to `trialing` or
// `signup`. Only ListExpiring ever passes true — expiringScope, and so
// CountExpiring, always passes false — see ListOptions.IncludeSignup for why
// a signup row must not reach the KPI. The false branch builds the same
// single-status predicate it always has, unchanged.
func trialingInWindowScope(db *gorm.DB, asOf time.Time, window time.Duration, includeSignup bool) *gorm.DB {
	scoped := db.Model(&subscription.StoreSubscription{})
	if includeSignup {
		scoped = scoped.Where("status IN ?", []subscription.SubscriptionStatus{
			subscription.StatusTrialing, subscription.StatusSignup,
		})
	} else {
		scoped = scoped.Where("status = ?", subscription.StatusTrialing)
	}
	return EndsBetweenScope(scoped, asOf, asOf.Add(window))
}

// listScope is the scope ListExpiring queries against. The two options widen
// two independent dimensions — which statuses, and whether card-backed rows
// count — so all four combinations are reachable and each means something:
//
//   - zero value: expiringScope, #285's shipped contract. Trialing, card-less.
//   - IncludeStripeManaged: drops the card filter only (#358). Still trialing.
//   - IncludeSignup: adds `signup` only. A card-backed signup row stays out.
//   - both: trialing or signup, card-backed or not.
//
// The zero value delegates to expiringScope rather than rebuilding the same
// predicate, so the default this list serves and the scope CountExpiring
// counts cannot drift apart.
func listScope(db *gorm.DB, asOf time.Time, window time.Duration, opts ListOptions) *gorm.DB {
	if !opts.IncludeSignup && !opts.IncludeStripeManaged {
		return expiringScope(db, asOf, window)
	}
	scoped := trialingInWindowScope(db, asOf, window, opts.IncludeSignup)
	if !opts.IncludeStripeManaged {
		scoped = scoped.Where("stripe_subscription_id IS NULL")
	}
	return scoped
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
func ListExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int, opts ListOptions) ([]ExpiringRow, int64, error) {
	// total is computed over THIS list's own scope, not via CountExpiring:
	// CountExpiring keeps the narrower "will expire" meaning for #282's KPI,
	// which must not move just because the list widened.
	var total int64
	err := listScope(db.WithContext(ctx), asOf, window, opts).Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("trial: count list expiring: %w", err)
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
	err = listScope(db.WithContext(ctx), asOf, window, opts).
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
			StripeManaged:    r.StripeSubscriptionID != nil && *r.StripeSubscriptionID != "",
		})
	}
	return rows, total, nil
}
