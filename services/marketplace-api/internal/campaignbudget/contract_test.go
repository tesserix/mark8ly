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

// TestRecomputeContract_P4CallerSemantics verifies the exact calling shape
// P4's change-plan orchestrator uses: pass the caller's *gorm.DB mid-tx,
// and the budget write joins that transaction atomically.
func TestRecomputeContract_P4CallerSemantics(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 5000, 5000)`, storeID, month).Error)

	svc := campaignbudget.NewService(db)

	// Shape that P4's change-plan handler will use.
	err := db.Transaction(func(tx *gorm.DB) error {
		// Imagine P4 wrote the plan row here.
		return svc.RecomputeLimitForPlan(context.Background(), tx, storeID, "studio")
	})
	require.NoError(t, err)

	var limitSet int
	require.NoError(t, db.Raw(
		`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&limitSet))
	require.Equal(t, 50000, limitSet)
}

// TestRecomputeContract_P4RollbackIsolation verifies that if the outer
// transaction rolls back (e.g. Stripe call fails after budget write),
// the budget write is also rolled back — plan row and budget stay in sync.
func TestRecomputeContract_P4RollbackIsolation(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 5000, 5000)`, storeID, month).Error)

	svc := campaignbudget.NewService(db)
	sentinel := errors.New("stripe-failed")

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := svc.RecomputeLimitForPlan(context.Background(), tx, storeID, "studio"); err != nil {
			return err
		}
		// Simulate Stripe failing after budget write.
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	var limitSet int
	require.NoError(t, db.Raw(
		`SELECT limit_set FROM campaign_email_budget WHERE store_id=$1`, storeID,
	).Row().Scan(&limitSet))
	require.Equal(t, 5000, limitSet, "rollback must revert the budget update together with the plan row")
}
