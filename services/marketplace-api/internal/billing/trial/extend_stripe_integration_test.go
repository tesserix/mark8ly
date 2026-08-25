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

var stripeExtendAsOf = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// A card-less trial must behave IDENTICALLY through the Extender as it did
// through the package function. This is the regression guard for the
// refactor: if the new code path changes the common support case, this fails.
func TestExtender_CardlessPathUnchanged(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedExpiringRow(t, db, derivedEnd, nil)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	res, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)
	require.True(t, derivedEnd.Equal(res.PreviousEndsAt))
	require.True(t, newEnd.Equal(res.NewEndsAt))
	require.False(t, res.StripeApplied, "a card-less extension touches no Stripe subscription")

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
}

// A card-backed trial on a build with NO Stripe configured must refuse
// exactly as it does today. This is the guarantee that an unconfigured pod
// can never silently extend a Stripe-managed trial locally.
func TestExtender_NilUpdater_RefusesCardBacked(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	subID := "sub_nil_updater"
	seeded := seedExpiringRow(t, db, stripeExtendAsOf.Add(10*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeManaged)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "a refused extension must write nothing")
}
