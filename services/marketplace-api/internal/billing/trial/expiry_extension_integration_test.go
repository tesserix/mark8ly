//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// THE test for this issue. A subscription created 100 days ago is past its
// derived 90-day end and the cron expires it today. Give it a trial_ends_at
// 30 days in the future and it must SURVIVE.
//
// Delete the extended branch of EndedBeforeScope and this fails — which is
// exactly what "an extension actually extends something" means.
func TestExpiryCron_DoesNotExpireAnExtendedTrial(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "audit_logs")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	created := now.Add(-100 * 24 * time.Hour)
	extended := now.Add(30 * 24 * time.Hour)

	// unextended: seedSub(t, db, created, nil) — effective end is
	// created + 90d, so seed directly at that effective end (unextended,
	// so seedExpiringRow's default CreatedAt = effective end - 90d lands
	// exactly back on `created`).
	unextended := seedExpiringRow(t, db, created.Add(trialLen), nil)
	// protected: seedSub(t, db, created, &extended) — created_at is
	// `created` but trial_ends_at is stored as `extended`, the effective end.
	protected := seedExpiringRow(t, db, extended, func(r *subscription.StoreSubscription) {
		r.CreatedAt = created
		r.TrialEndsAt = &extended
	})

	cron := trial.NewExpiryCron(db, nil, nil, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "id = ?", protected.ID).Error)
	require.Equal(t, subscription.StatusTrialing, after.Status,
		"an extended trial must not be expired by the day-90 rule")

	var control subscription.StoreSubscription
	require.NoError(t, db.First(&control, "id = ?", unextended.ID).Error)
	require.Equal(t, subscription.StatusExpired, control.Status,
		"the unextended control MUST expire — otherwise this test passes because the cron did nothing")
}

// An extended trial appears in the expiring window at its NEW end, and is
// absent from the window around its old one.
func TestListExpiring_UsesTheExtendedEnd(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	created := now.Add(-88 * 24 * time.Hour) // derived end is 2 days away
	extended := now.Add(40 * 24 * time.Hour) // real end is 40 days away
	// seedSub(t, db, created, &extended): created_at is `created`,
	// trial_ends_at stored as `extended` (the effective end).
	seedExpiringRow(t, db, extended, func(r *subscription.StoreSubscription) {
		r.CreatedAt = created
		r.TrialEndsAt = &extended
	})

	// A 7-day window around now would catch the DERIVED end and must not.
	rows, total, err := trial.ListExpiring(context.Background(), db, now, 7*24*time.Hour, 1, 50, trial.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, int64(0), total, "the old derived end must not put an extended trial in the window")
	require.Empty(t, rows)

	// A window that reaches the NEW end must catch it, and report that date.
	rows, total, err = trial.ListExpiring(context.Background(), db, now, 45*24*time.Hour, 1, 50, trial.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, extended.UTC(), rows[0].TrialEndsAt,
		"the row must report its effective end, not created_at + 90d")
}

// ListExpiring orders by soonest-ending first. Before #353 it ordered by
// created_at on the assumption that every row shared one trial length;
// extensions break that, so an older row with a later extended end must sort
// AFTER a newer row ending sooner.
func TestListExpiring_OrdersByEffectiveEndNotCreatedAt(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	// Older row, extended to end LATER.
	// seedSub(t, db, now.Add(-120*24h), &laterEnd): created_at is
	// now-120d, trial_ends_at stored as laterEnd (the effective end).
	laterEnd := now.Add(20 * 24 * time.Hour)
	older := seedExpiringRow(t, db, laterEnd, func(r *subscription.StoreSubscription) {
		r.CreatedAt = now.Add(-120 * 24 * time.Hour)
		r.TrialEndsAt = &laterEnd
	})
	// Newer row, unextended, ending SOONER.
	// seedSub(t, db, now.Add(-85*24h), nil) ends in 5 days: effective end
	// is created + 90d = now + 5d, so seed directly at that effective end.
	sooner := seedExpiringRow(t, db, now.Add(-85*24*time.Hour).Add(trialLen), nil) // ends in 5 days

	rows, total, err := trial.ListExpiring(context.Background(), db, now, 30*24*time.Hour, 1, 50, trial.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	require.Equal(t, sooner.StoreID.String(), rows[0].StoreID,
		"soonest effective end must come first; ordering by created_at would invert these")
	require.Equal(t, older.StoreID.String(), rows[1].StoreID)
}
