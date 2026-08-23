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

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10)
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

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10)
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

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10)
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

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 1000)
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

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 1)
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

	rows, total, err := trial.ListExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow, 1, 10)
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
	assert.True(t, row.HasPaymentMethod)
	assert.Equal(t, string(subscription.StatusTrialing), row.Status)

	rowNoPM := rows[1]
	assert.Equal(t, seededNoPM.StoreID.String(), rowNoPM.StoreID)
	assert.False(t, rowNoPM.HasPaymentMethod)
}
