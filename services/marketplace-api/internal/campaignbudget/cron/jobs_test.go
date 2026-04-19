//go:build integration

package cron_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget/cron"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func firstOfMonthUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func TestTrialRampJob_AppliesToTransitioningStores(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID := uuid.New()

	// Store A: day 4 today → should ramp (created 3 days ago).
	storeA := uuid.New()
	sigA := time.Now().UTC().AddDate(0, 0, -3)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, created_at)
		VALUES ($1, $2, 'cus_a', 'trial', 'trialing', $3)`, tenantID, storeA, sigA).Error)
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 500, 500)`, storeA, month).Error)

	// Store B: day 2 today → no-op (created 1 day ago).
	storeB := uuid.New()
	sigB := time.Now().UTC().AddDate(0, 0, -1)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, created_at)
		VALUES ($1, $2, 'cus_b', 'trial', 'trialing', $3)`, tenantID, storeB, sigB).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 300, 500)`, storeB, month).Error)

	require.NoError(t, cron.RunTrialRampOnce(context.Background(), db, time.Now()))

	var limitA, limitB int
	require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeA).Row().Scan(&limitA))
	require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeB).Row().Scan(&limitB))
	require.Equal(t, 2000, limitA, "day-4 store ramped to 2000")
	require.Equal(t, 500, limitB, "day-2 store unchanged")
}

func TestTrialRampJob_SkipsNonTrialingStatuses(t *testing.T) {
	// Only 'signup' and 'trialing' stores should be queried.
	db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
	tenantID, storeID := uuid.New(), uuid.New()
	// Store is 'active' so must be skipped by the cron even if day=4.
	createdAt := time.Now().UTC().AddDate(0, 0, -3)
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, created_at)
		VALUES ($1, $2, 'cus_x', 'starter', 'active', $3)`, tenantID, storeID, createdAt).Error)
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 500, 500)`, storeID, month).Error)

	require.NoError(t, cron.RunTrialRampOnce(context.Background(), db, time.Now()))

	var limitSet int
	require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID).Row().Scan(&limitSet))
	require.Equal(t, 500, limitSet, "active store must not be ramped by trial cron")
}
