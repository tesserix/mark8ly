//go:build integration

package campaignbudget_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestMonthlyReset_SeedsAllActiveSubscriptions(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID := uuid.New()
	storeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, sid := range storeIDs {
		testdb.SeedStore(t, db, tenantID, sid)
		require.NoError(t, db.Exec(`
			INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
			VALUES ($1, $2, $3, 'starter', 'active')`, tenantID, sid, "cus_"+sid.String()).Error)
	}

	require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

	month := firstOfMonthUTC(time.Now())
	for _, sid := range storeIDs {
		var remaining, limitSet int
		err := db.Raw(`
			SELECT remaining, limit_set FROM campaign_email_budget
			WHERE store_id = $1 AND month = $2`, sid, month,
		).Row().Scan(&remaining, &limitSet)
		require.NoError(t, err)
		require.Equal(t, 15000, limitSet, "starter allowance per spec §9")
		require.Equal(t, 15000, remaining)
	}
}

// A trial store must start at the §5.1 D1-3 tier of 500, NOT at the 5000
// trial allowance. Seeding at the allowance defeated the ramp entirely: the
// day-4 step raises the ceiling to 2000, which can never raise a ceiling
// already at 5000, so a brand-new store had its whole month's quota on day
// one and the documented D4-7 tier never applied to anyone (#424).
func TestMonthlyReset_SeedsTrialAtTheD1To3Tier(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID := uuid.New()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
		VALUES ($1, $2, $3, 'trial', 'trialing')`, tenantID, storeID, "cus_"+storeID.String()).Error)

	require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

	var remaining, limitSet int
	require.NoError(t, db.Raw(`
		SELECT remaining, limit_set FROM campaign_email_budget
		WHERE store_id = $1 AND month = $2`, storeID, firstOfMonthUTC(time.Now()),
	).Row().Scan(&remaining, &limitSet))

	require.Equal(t, 500, limitSet, "trial seeds at the D1-3 tier, not the 5000 allowance (#424)")
	require.Equal(t, 500, remaining)

	// And the ramp must then be able to raise it — proving the tier is a
	// floor to climb from, not a cap the ramp cannot move.
	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial"))
	require.NoError(t, db.Raw(`
		SELECT remaining, limit_set FROM campaign_email_budget
		WHERE store_id = $1 AND month = $2`, storeID, firstOfMonthUTC(time.Now()),
	).Row().Scan(&remaining, &limitSet))
	require.Equal(t, 2000, limitSet, "day-4 raises the ceiling to the D4-7 tier")
	require.Equal(t, 2000, remaining)
}

// The day-4 ceiling must come from limit_set, never from remaining. A store
// that has SPENT its balance down must not have its ceiling cut as a result
// — under `GREATEST(remaining, 2000)` this row's ceiling collapsed from 5000
// to 2000 purely because the merchant used their quota, while an identical
// store that had sent nothing kept 5000 (#424).
//
// It also pins that the step only ever RAISES: a row seeded at the old full
// allowance must not be cut to the 2000 tier on day 4.
func TestApplyTrialRamp_Day4CeilingIgnoresConsumption(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	// A legacy row seeded at the full allowance, mostly spent.
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 100, 5000)`, storeID, month).Error)

	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial"))

	var remaining, limitSet int
	require.NoError(t, db.Raw(`
		SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id = $1`, storeID,
	).Row().Scan(&remaining, &limitSet))

	require.Equal(t, 5000, limitSet,
		"the ceiling must not be cut because the merchant spent: derive it from limit_set, not remaining (#424)")
	require.Equal(t, 2000, remaining, "the balance is still raised to the D4-7 floor")
}

func TestMonthlyReset_IdempotentReRun(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID := uuid.New()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
		VALUES ($1, $2, 'cus_x', 'starter', 'active')`, tenantID, storeID).Error)

	require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))
	// Consume some.
	_, err := campaignbudget.Reserve(context.Background(), db, storeID, 5000)
	require.NoError(t, err)
	// Re-run same day — must NOT overwrite consumed state.
	require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

	var remaining int
	require.NoError(t, db.Raw(`
		SELECT remaining FROM campaign_email_budget
		WHERE store_id = $1 AND month = date_trunc('month', (now() AT TIME ZONE 'utc'))::date`,
		storeID).Row().Scan(&remaining))
	require.Equal(t, 10000, remaining, "re-run must NOT reset remaining back up")
}

func TestMonthlyReset_SkipsNonActiveStatuses(t *testing.T) {
	// expired / store_closed / pending_hard_delete subscriptions get no row.
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID, activeID, expiredID := uuid.New(), uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, activeID)
	testdb.SeedStore(t, db, tenantID, expiredID)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
		VALUES ($1, $2, 'cus_a', 'starter', 'active'),
		       ($1, $3, 'cus_e', 'starter', 'expired')`,
		tenantID, activeID, expiredID).Error)

	require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

	month := firstOfMonthUTC(time.Now())
	var activeExists, expiredExists bool
	require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM campaign_email_budget WHERE store_id=$1 AND month=$2)`, activeID, month).Row().Scan(&activeExists))
	require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM campaign_email_budget WHERE store_id=$1 AND month=$2)`, expiredID, month).Row().Scan(&expiredExists))
	require.True(t, activeExists)
	require.False(t, expiredExists)
}

func TestMonthlyReset_SkipsPro_Negotiated(t *testing.T) {
	// Pro stores are excluded — ops sets limit_set manually.
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID, proID, starterID := uuid.New(), uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, proID)
	testdb.SeedStore(t, db, tenantID, starterID)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
		VALUES ($1, $2, 'cus_p', 'pro', 'active'),
		       ($1, $3, 'cus_s', 'starter', 'active')`,
		tenantID, proID, starterID).Error)

	require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

	month := firstOfMonthUTC(time.Now())
	var proExists, starterExists bool
	require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM campaign_email_budget WHERE store_id=$1 AND month=$2)`, proID, month).Row().Scan(&proExists))
	require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM campaign_email_budget WHERE store_id=$1 AND month=$2)`, starterID, month).Row().Scan(&starterExists))
	require.False(t, proExists, "pro (negotiated) must be excluded from monthly reset")
	require.True(t, starterExists)
}
