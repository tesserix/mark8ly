//go:build integration

package transactional_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget/transactional"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func firstOfMonthUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func TestTransactionalCounter_Record_IncrementsOrInserts(t *testing.T) {
	db := testdb.NewDB(t, "store_transactional_counter")
	storeID := uuid.New()

	count, err := transactional.Record(context.Background(), db, storeID, 50)
	require.NoError(t, err)
	require.Equal(t, 50, count)

	count, err = transactional.Record(context.Background(), db, storeID, 30)
	require.NoError(t, err)
	require.Equal(t, 80, count)
}

func TestTransactionalCounter_FairUseCap_SoftWarning(t *testing.T) {
	db := testdb.NewDB(t, "store_transactional_counter")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO store_transactional_counter (store_id, month, sent)
		VALUES ($1, $2, 99990)`, storeID, month).Error)

	// Recording 20 more still succeeds (soft cap) but flags overage.
	count, err := transactional.Record(context.Background(), db, storeID, 20)
	require.NoError(t, err)
	require.Equal(t, 100010, count)
	require.True(t, transactional.IsOverFairUse(count), "100k+ flagged for ops review")
}

func TestIsOverFairUse_Boundary(t *testing.T) {
	require.False(t, transactional.IsOverFairUse(100_000), "exactly at cap is not over")
	require.True(t, transactional.IsOverFairUse(100_001), "one over cap is flagged")
}

func TestTransactionalCounter_ZeroCount_NoOp(t *testing.T) {
	db := testdb.NewDB(t, "store_transactional_counter")
	storeID := uuid.New()

	count, err := transactional.Record(context.Background(), db, storeID, 0)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}
