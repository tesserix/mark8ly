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

func TestMonthlyReset_IdempotentReRun(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID := uuid.New()
	storeID := uuid.New()
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
