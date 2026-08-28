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

func TestApplyTrialRamp_Day3To4_RaisesToCeiling(t *testing.T) {
	// Day 3→4: limit_set = GREATEST(remaining, 2000), remaining = same.
	// Merchant sent some emails: remaining=50, limit_set=500.
	// After ramp: limit_set=2000, remaining=2000 (raised to new ceiling).
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 50, 500)`, storeID, month).Error)

	err := campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial")
	require.NoError(t, err)

	var remaining, limitSet int
	require.NoError(t, db.Raw(
		`SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&remaining, &limitSet))
	require.Equal(t, 2000, limitSet)
	require.Equal(t, 2000, remaining, "day-4 ramp raises remaining to new ceiling")
}

func TestApplyTrialRamp_Day7To8_UsesPlanAllowance(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 1500, 2000)`, storeID, month).Error)

	// Trial plan allowance = 5000 per spec §9.
	err := campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 8, "trial")
	require.NoError(t, err)

	var limitSet int
	require.NoError(t, db.Raw(
		`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&limitSet))
	require.Equal(t, 5000, limitSet)
}

func TestApplyTrialRamp_NonTransitionDay_NoOp(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 300, 500)`, storeID, month).Error)

	// Day 2 is not a transition day.
	err := campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 2, "trial")
	require.NoError(t, err)

	var remaining, limitSet int
	require.NoError(t, db.Raw(
		`SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&remaining, &limitSet))
	require.Equal(t, 300, remaining)
	require.Equal(t, 500, limitSet)
}

func TestApplyTrialRamp_Idempotent_ReRunSameDay(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 50, 500)`, storeID, month).Error)

	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial"))
	// Merchant consumes some.
	_, err := campaignbudget.Reserve(context.Background(), db, storeID, 200)
	require.NoError(t, err)
	// Cron re-runs (pod restart). Must NOT reset remaining back up.
	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial"))

	var remaining, limitSet int
	require.NoError(t, db.Raw(
		`SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&remaining, &limitSet))
	require.Equal(t, 1800, remaining, "idempotent: re-running the ramp must not re-inflate remaining")
	require.Equal(t, 2000, limitSet)
}

func TestApplyTrialRamp_Day8_Idempotent_ReRunSameDay(t *testing.T) {
	// Day 8 has the same GREATEST shape as day 4 and the same defect (#399):
	// ramp to the plan allowance, spend some of it, re-run — the balance must
	// NOT climb back to the allowance.
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 1500, 2000)`, storeID, month).Error)

	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 8, "trial"))

	var afterRamp int
	require.NoError(t, db.Raw(
		`SELECT remaining FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&afterRamp))

	// Consume a chunk of the ramped budget.
	require.NoError(t, db.Exec(`
		UPDATE campaign_email_budget SET remaining = remaining - 3000
		WHERE store_id = $1`, storeID).Error)

	require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 8, "trial"))

	var remaining int
	require.NoError(t, db.Raw(
		`SELECT remaining FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&remaining))
	require.Equal(t, afterRamp-3000, remaining,
		"idempotent: re-running the day-8 ramp must not re-inflate remaining")
}
