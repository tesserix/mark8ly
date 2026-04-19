//go:build integration

package campaignbudget_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func firstOfMonthUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func TestReserve_HappyPath(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())

	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 5000, 5000)`, storeID, month).Error)

	remaining, err := campaignbudget.Reserve(context.Background(), db, storeID, 100)
	require.NoError(t, err)
	require.Equal(t, 4900, remaining)
}

func TestReserve_Exhausted(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 50, 5000)`, storeID, month).Error)

	_, err := campaignbudget.Reserve(context.Background(), db, storeID, 100)
	require.ErrorIs(t, err, campaignbudget.ErrBudgetExhausted)

	// Row must be unchanged — atomic UPDATE never took effect.
	var remaining int
	require.NoError(t, db.Raw(`SELECT remaining FROM campaign_email_budget WHERE store_id=$1`, storeID).Scan(&remaining).Error)
	require.Equal(t, 50, remaining)
}

func TestReserve_NoRow(t *testing.T) {
	db := testdb.NewDB(t, "campaign_email_budget")
	_, err := campaignbudget.Reserve(context.Background(), db, uuid.New(), 10)
	require.ErrorIs(t, err, campaignbudget.ErrNoBudgetRow)
}

func TestReserve_ExactMatch(t *testing.T) {
	// recipient_count == remaining → success, resulting remaining = 0.
	db := testdb.NewDB(t, "campaign_email_budget")
	storeID := uuid.New()
	month := firstOfMonthUTC(time.Now())
	require.NoError(t, db.Exec(`
		INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
		VALUES ($1, $2, 100, 5000)`, storeID, month).Error)

	remaining, err := campaignbudget.Reserve(context.Background(), db, storeID, 100)
	require.NoError(t, err)
	require.Equal(t, 0, remaining)

	// Next 1-recipient send must now fail.
	_, err = campaignbudget.Reserve(context.Background(), db, storeID, 1)
	require.True(t, errors.Is(err, campaignbudget.ErrBudgetExhausted))
}
