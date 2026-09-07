//go:build integration

package trial_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// expiringAsOf is fixed (not time.Now()) so the window-boundary assertions
// below stay stable forever instead of drifting with the calendar.
var expiringAsOf = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

var trialLen = time.Duration(trial.TrialDays) * 24 * time.Hour

// seedExpiringStore inserts a minimal stores row so a store_subscriptions
// row referencing storeID satisfies store_subscriptions_store_id_fkey.
// Mirrors internal/subscription/repository_kpi_integration_test.go's helper.
func seedExpiringStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// seedExpiringRow inserts a trialing (by default) StoreSubscription whose
// created_at is set so its trial ends at trialEndsAt (created_at =
// trialEndsAt - TrialDays). Overrides can be applied via opts.
func seedExpiringRow(t *testing.T, db *gorm.DB, trialEndsAt time.Time, opts func(*subscription.StoreSubscription)) subscription.StoreSubscription {
	t.Helper()

	tenantID := uuid.New()
	storeID := uuid.New()
	seedExpiringStore(t, db, tenantID, storeID)

	row := subscription.StoreSubscription{
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_expiring_" + trialEndsAt.Format("20060102150405"),
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CurrentPeriodEnd:   nil,
		CreatedAt:          trialEndsAt.Add(-trialLen),
	}
	if opts != nil {
		opts(&row)
	}
	return seedSubscription(t, db, row)
}

// TestListExpiring_CardlessTrialEndingSoon proves a card-less trial ending
// in 3 days both counts and appears in the listing.
func TestListExpiring_CardlessTrialEndingSoon(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, string(subscription.StatusTrialing), rows[0].Status)
}

// TestListExpiring_ExcludesRowsWithStripeSubscription is the assertion that
// would have caught the original defect: a trial WITH a card (stripe
// subscription set) must not count even though its created_at puts its
// trial end inside the window — it will convert, not expire.
func TestListExpiring_ExcludesRowsWithStripeSubscription(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	subID := "sub_has_card"
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), func(r *subscription.StoreSubscription) {
		r.StripeSubscriptionID = &subID
	})

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, rows)
}

// TestListExpiring_IncludesNullCurrentPeriodEnd proves the population the
// old CountTrialsExpiring silently dropped — a row with current_period_end
// explicitly NULL — still appears here.
func TestListExpiring_IncludesNullCurrentPeriodEnd(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), func(r *subscription.StoreSubscription) {
		r.CurrentPeriodEnd = nil
	})

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// TestListExpiring_RightEdgeInclusive proves a trial ending exactly at
// asOf+window counts.
func TestListExpiring_RightEdgeInclusive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(trial.DefaultExpiryWindow), nil)

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// TestListExpiring_LeftEdgeExclusive proves a trial ending exactly at asOf
// (already expired) does not count — half-open left.
func TestListExpiring_LeftEdgeExclusive(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf, nil)

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// TestListExpiring_BeyondWindowExcluded proves a trial ending past the
// window does not count.
func TestListExpiring_BeyondWindowExcluded(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(trial.DefaultExpiryWindow+time.Hour), nil)

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// TestListExpiring_ActiveStatusExcluded proves status=active never counts,
// however recent its created_at.
func TestListExpiring_ActiveStatusExcluded(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), func(r *subscription.StoreSubscription) {
		r.Status = subscription.StatusActive
	})

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// TestListExpiring_OrderedSoonestFirst seeds three rows whose insertion
// order differs from expiry order and asserts the listing comes back
// soonest-first.
func TestListExpiring_OrderedSoonestFirst(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	rowLatest := seedExpiringRow(t, db, expiringAsOf.Add(6*24*time.Hour), nil)
	rowSoonest := seedExpiringRow(t, db, expiringAsOf.Add(1*24*time.Hour), nil)
	rowMiddle := seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, rows, 3)

	assert.Equal(t, rowSoonest.StoreID.String(), rows[0].StoreID)
	assert.Equal(t, rowMiddle.StoreID.String(), rows[1].StoreID)
	assert.Equal(t, rowLatest.StoreID.String(), rows[2].StoreID)
}

// TestListExpiring_CountMatchesListLength proves CountExpiring equals
// len(rows) from ListExpiring for the same asOf/window with a large limit.
func TestListExpiring_CountMatchesListLength(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(1*24*time.Hour), nil)
	seedExpiringRow(t, db, expiringAsOf.Add(2*24*time.Hour), nil)
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)

	count, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 1000, trial.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, count, total)
	assert.Equal(t, int(count), len(rows))
}

// TestListExpiring_Pagination proves limit=1 returns one row and a total
// reflecting the full match count.
func TestListExpiring_Pagination(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedExpiringRow(t, db, expiringAsOf.Add(1*24*time.Hour), nil)
	seedExpiringRow(t, db, expiringAsOf.Add(2*24*time.Hour), nil)
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 1, trial.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, rows, 1)
}

// TestListExpiring_MapsAllFields seeds a row with distinct, non-default
// values for every field ListExpiring maps onto ExpiringRow and asserts
// each one individually. Without this, a stub-driven handler test cannot
// catch a broken row-to-struct mapping — nothing else in the suite reads a
// real row out of the database and checks its fields one by one.
//
// HasPaymentMethod is seeded true here even though every row returned by
// this query has stripe_subscription_id IS NULL (and so is false in
// practice today, per the spec's "deliberate redundancy" note). Nothing in
// the schema or expiringScope ties has_default_payment_method to
// stripe_subscription_id, so seeding it true is exactly what proves the
// value is READ from the column rather than inferred from the filter
// (e.g. hardcoded true, or hardcoded false "because these rows never have
// a card").
func TestListExpiring_MapsAllFields(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	trialEndsAt := expiringAsOf.Add(3 * 24 * time.Hour)
	currency := "GBP"

	seeded := seedExpiringRow(t, db, trialEndsAt, func(r *subscription.StoreSubscription) {
		r.Plan = subscription.PlanStudio
		r.SubscriptionPeriod = subscription.PeriodAnnual
		r.BillingCurrency = &currency
		r.PriceTier = subscription.PriceTierPPP
		r.HasDefaultPaymentMethod = true
	})

	// A second row, seeded HasDefaultPaymentMethod=false at a different
	// trial end, so a hardcoded-true mapping (as well as a hardcoded-false
	// one) is caught: the true-seeded row above proves the value is READ
	// from the column rather than inferred false from the selection filter,
	// and this row proves it isn't simply hardcoded true either.
	seededNoPM := seedExpiringRow(t, db, expiringAsOf.Add(5*24*time.Hour), func(r *subscription.StoreSubscription) {
		r.HasDefaultPaymentMethod = false
	})

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	row := rows[0]
	assert.Equal(t, seeded.TenantID.String(), row.TenantID)
	assert.Equal(t, seeded.StoreID.String(), row.StoreID)
	assert.True(t, trialEndsAt.Equal(row.TrialEndsAt),
		"TrialEndsAt: want %s, got %s", trialEndsAt, row.TrialEndsAt)
	// Computed independently of the production expression, per the spec:
	// "trial_ends_at equals created_at + TrialDays".
	wantTrialEndsAt := seeded.CreatedAt.Add(time.Duration(trial.TrialDays) * 24 * time.Hour)
	assert.True(t, wantTrialEndsAt.Equal(row.TrialEndsAt),
		"TrialEndsAt vs created_at+TrialDays: want %s, got %s", wantTrialEndsAt, row.TrialEndsAt)
	assert.Equal(t, string(subscription.PlanStudio), row.Plan)
	assert.Equal(t, string(subscription.PeriodAnnual), row.Period)
	require.NotNil(t, row.BillingCurrency)
	assert.Equal(t, "GBP", *row.BillingCurrency)
	assert.Equal(t, subscription.PriceTierPPP, row.PriceTier)
	assert.True(t, row.HasPaymentMethod)
	assert.Equal(t, string(subscription.StatusTrialing), row.Status)

	rowNoPM := rows[1]
	assert.Equal(t, seededNoPM.StoreID.String(), rowNoPM.StoreID)
	assert.False(t, rowNoPM.HasPaymentMethod)
}

// The default must be unchanged: a card-backed trial is not "expiring".
// This is the contract #285 already ships.
func TestListExpiring_DefaultStillExcludesStripeManaged(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	subID := "sub_listed"
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })
	seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "the card-backed row must not appear by default")
	require.Len(t, rows, 1)
	require.False(t, rows[0].StripeManaged)
}

// With the flag, BOTH appear and each is labelled. Two rows of different
// kinds in one fixture: one kind cannot prove a filter.
func TestListExpiring_IncludeStripeManaged_ReturnsBothLabelled(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	subID := "sub_listed_2"
	cardBacked := seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })
	cardLess := seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{IncludeStripeManaged: true})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	byStore := map[string]trial.ExpiringRow{}
	for _, r := range rows {
		byStore[r.StoreID] = r
	}
	require.True(t, byStore[cardBacked.StoreID.String()].StripeManaged)
	require.False(t, byStore[cardLess.StoreID.String()].StripeManaged)
}

// The KPI must NOT move. CountExpiring keeps its "will expire" meaning.
func TestCountExpiring_UnaffectedByStripeManagedRows(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	subID := "sub_kpi"
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })
	seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	n, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "trials_expiring counts trials that EXPIRE; a card-backed trial converts")
}

// seedSignupRow inserts a `signup` subscription whose trial has never been
// extended, so its effective end comes from the null-trial_ends_at branch:
// created_at + TrialDays. That is the population Bootstrap creates and the
// checkout webhook never moved on — see trial.ListOptions.IncludeSignup.
func seedSignupRow(t *testing.T, db *gorm.DB, trialEndsAt time.Time, opts func(*subscription.StoreSubscription)) subscription.StoreSubscription {
	t.Helper()

	return seedExpiringRow(t, db, trialEndsAt, func(r *subscription.StoreSubscription) {
		r.Status = subscription.StatusSignup
		if opts != nil {
			opts(r)
		}
	})
}

// The default must be unchanged: a tenant still at `signup` is not
// "expiring". This is the contract #285 already ships, and the zero
// ListOptions value must keep it byte-for-byte.
func TestListExpiring_DefaultExcludesSignup(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedSignupRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)
	trialing := seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "the signup row must not appear by default")
	require.Len(t, rows, 1)
	require.Equal(t, trialing.StoreID.String(), rows[0].StoreID)
	require.Equal(t, string(subscription.StatusTrialing), rows[0].Status)
}

// With the flag, BOTH appear and Status is what tells them apart — an
// operator chases a `signup` tenant to complete checkout, but a `trialing`
// one is about to lose access. Two rows of different kinds in one fixture:
// one kind cannot prove a filter.
func TestListExpiring_IncludeSignup_ReturnsBothLabelledByStatus(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	signup := seedSignupRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)
	trialing := seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{IncludeSignup: true})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	byStore := map[string]trial.ExpiringRow{}
	for _, r := range rows {
		byStore[r.StoreID] = r
	}
	require.Equal(t, string(subscription.StatusSignup), byStore[signup.StoreID.String()].Status)
	require.Equal(t, string(subscription.StatusTrialing), byStore[trialing.StoreID.String()].Status)
}

// The two options are independent, and all four combinations are
// meaningful. One fixture holds a row of each kind — trialing/signup ×
// card-less/card-backed — so a combination that admits one axis by
// accident when widening the other is caught.
//
// IncludeSignup alone must not drag a card-backed row in, and
// IncludeStripeManaged alone must not drag a signup row in.
func TestListExpiring_SignupAndStripeManagedCompose(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	cardSub := "sub_compose_trialing"
	signupCardSub := "sub_compose_signup"

	trialingCardless := seedExpiringRow(t, db, expiringAsOf.Add(1*24*time.Hour), nil)
	trialingCarded := seedExpiringRow(t, db, expiringAsOf.Add(2*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.StripeSubscriptionID = &cardSub })
	signupCardless := seedSignupRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)
	signupCarded := seedSignupRow(t, db, expiringAsOf.Add(4*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.StripeSubscriptionID = &signupCardSub })

	cases := []struct {
		name string
		opts trial.ListOptions
		want []string
	}{
		{
			name: "zero value: trialing and card-less only",
			opts: trial.ListOptions{},
			want: []string{trialingCardless.StoreID.String()},
		},
		{
			name: "IncludeStripeManaged alone drops the card filter only",
			opts: trial.ListOptions{IncludeStripeManaged: true},
			want: []string{trialingCardless.StoreID.String(), trialingCarded.StoreID.String()},
		},
		{
			name: "IncludeSignup alone widens the status set only",
			opts: trial.ListOptions{IncludeSignup: true},
			want: []string{trialingCardless.StoreID.String(), signupCardless.StoreID.String()},
		},
		{
			name: "both widen both axes",
			opts: trial.ListOptions{IncludeSignup: true, IncludeStripeManaged: true},
			want: []string{
				trialingCardless.StoreID.String(), trialingCarded.StoreID.String(),
				signupCardless.StoreID.String(), signupCarded.StoreID.String(),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := trial.ListExpiring(context.Background(), db,
				expiringAsOf, trial.DefaultExpiryWindow, 1, 10, tc.opts)
			require.NoError(t, err)
			require.Equal(t, int64(len(tc.want)), total)

			got := make([]string, 0, len(rows))
			for _, r := range rows {
				got = append(got, r.StoreID)
			}
			require.ElementsMatch(t, tc.want, got)
		})
	}
}

// A signup row has no trial_ends_at, so its effective end comes from
// created_at + TrialDays — the same fallback EndsAt applies. It must sort
// against a trialing row's EXPLICIT trial_ends_at on that effective date,
// not on created_at and not after every extended row.
//
// The extended trialing row is seeded with a created_at far outside the
// window on purpose: if the ordering (or the window predicate) read
// created_at rather than the effective end, this row would sort first, or
// vanish from the page entirely.
func TestListExpiring_SignupRowSortsOnItsDerivedEnd(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	extendedEnd := expiringAsOf.Add(2 * 24 * time.Hour)
	extended := seedExpiringRow(t, db, expiringAsOf.Add(300*24*time.Hour),
		func(r *subscription.StoreSubscription) {
			r.CreatedAt = expiringAsOf.Add(-200 * 24 * time.Hour)
			r.TrialEndsAt = &extendedEnd
		})
	signupEarly := seedSignupRow(t, db, expiringAsOf.Add(1*24*time.Hour), nil)
	signupLate := seedSignupRow(t, db, expiringAsOf.Add(5*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{IncludeSignup: true})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, rows, 3)

	require.Equal(t, signupEarly.StoreID.String(), rows[0].StoreID, "day 1, derived from created_at + TrialDays")
	require.Equal(t, extended.StoreID.String(), rows[1].StoreID, "day 2, from an explicit trial_ends_at")
	require.Equal(t, signupLate.StoreID.String(), rows[2].StoreID, "day 5, derived")

	// The derived end is also what the row REPORTS, not created_at.
	require.True(t, expiringAsOf.Add(1*24*time.Hour).Equal(rows[0].TrialEndsAt),
		"want %s, got %s", expiringAsOf.Add(1*24*time.Hour), rows[0].TrialEndsAt)
}

// THE assertion that matters most. CountExpiring backs /admin/kpis's
// trials_expiring, which the console shares with this list "so the two
// cannot report different numbers for the same word". A `signup` row will
// NOT expire — expiry_cron selects status = 'trialing' only — so widening
// the list must leave the counter exactly where it was. CountExpiring takes
// no ListOptions at all, and this proves the flag cannot reach it by any
// other route either.
func TestCountExpiring_UnmovedByIncludeSignup(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	seedSignupRow(t, db, expiringAsOf.Add(3*24*time.Hour), nil)
	seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	n, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	require.Equal(t, int64(1), n,
		"trials_expiring counts trials that EXPIRE; a signup row never reaches the expiry cron")

	// The same fixture, listed with the flag, returns two. The counter and
	// the list are ALLOWED to differ here — that is the point — but only
	// because the list was asked an explicitly different question.
	_, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{IncludeSignup: true})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
}
