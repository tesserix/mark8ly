//go:build integration

package campaignbudget_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestRecomputeLimitForPlan_InsideCallerTransaction(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 3000, 5000)`, storeID, month).Error)

	// Simulate P4's change-plan transaction: write new plan + recompute in one tx.
	err := db.Transaction(func(tx *gorm.DB) error {
		return campaignbudget.RecomputeLimitForPlan(context.Background(), tx, storeID, "starter")
	})
	require.NoError(t, err)

	var limitSet int
	require.NoError(t, db.Raw(
		`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&limitSet))
	require.Equal(t, 15000, limitSet, "starter plan allowance per spec §9")
}

func TestRecomputeLimitForPlan_PlanNegotiated_NoOp(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 10000, 50000)`, storeID, month).Error)

	err := campaignbudget.RecomputeLimitForPlan(context.Background(), db, storeID, "pro")
	require.NoError(t, err, "Pro (negotiated) must not error, only warn + noop")

	var limitSet int
	require.NoError(t, db.Raw(
		`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&limitSet))
	require.Equal(t, 50000, limitSet, "limit_set unchanged when plan is Negotiated")
}

func TestRecomputeLimitForPlan_TxRollback_UndoesBudgetWrite(t *testing.T) {
	// Critical invariant: if the caller transaction rolls back, the budget
	// update must also roll back. Otherwise plan-row and budget-row state diverge.
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 3000, 5000)`, storeID, month).Error)

	sentinel := errors.New("rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := campaignbudget.RecomputeLimitForPlan(context.Background(), tx, storeID, "studio"); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	var limitSet int
	require.NoError(t, db.Raw(
		`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&limitSet))
	require.Equal(t, 5000, limitSet, "rollback must revert the budget update too")
}

func TestRecomputeLimitForPlan_Downgrade_ClampsRemaining(t *testing.T) {
	// Studio (50k) → Starter (15k): remaining must be clamped to 15k.
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 40000, 50000)`, storeID, month).Error)

	err := campaignbudget.RecomputeLimitForPlan(context.Background(), db, storeID, "starter")
	require.NoError(t, err)

	var remaining, limitSet int
	require.NoError(t, db.Raw(
		`SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&remaining, &limitSet))
	require.Equal(t, 15000, limitSet)
	require.Equal(t, 15000, remaining, "downgrade must clamp remaining to new limit")
}
