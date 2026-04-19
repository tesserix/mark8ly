//go:build integration

package seaqueue_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax/seaqueue"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestSEAQueue_EnqueueIsIdempotentPerStoreCountry(t *testing.T) {
	db := testdb.NewDB(t, "sea_manual_review_queue")
	repo := seaqueue.New(db)
	tenantID, storeID := uuid.New(), uuid.New()

	first, err := repo.Enqueue(context.Background(), seaqueue.Entry{
		TenantID: tenantID, StoreID: storeID, Country: "MY",
		TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
		QueueReason: "mof_sst_manual",
	})
	require.NoError(t, err)
	require.Equal(t, seaqueue.StatusPending, first.Status)

	second, err := repo.Enqueue(context.Background(), seaqueue.Entry{
		TenantID: tenantID, StoreID: storeID, Country: "MY",
		TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
		QueueReason: "mof_sst_manual",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "duplicate enqueue must return original row")
}

func TestSEAQueue_AddBusinessDays_SkipsWeekends(t *testing.T) {
	monday := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC), seaqueue.AddBusinessDays(monday, 5))

	// Tuesday + 5 biz days = next Tuesday.
	tuesday := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC), seaqueue.AddBusinessDays(tuesday, 5))

	// Friday + 1 biz day = next Monday (skip Sat/Sun).
	friday := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC), seaqueue.AddBusinessDays(friday, 1))
}

func TestSEAQueue_Resolve_ApprovedThenIdempotent(t *testing.T) {
	db := testdb.NewDB(t, "sea_manual_review_queue")
	repo := seaqueue.New(db)
	reviewerID := uuid.New()

	entry, err := repo.Enqueue(context.Background(), seaqueue.Entry{
		TenantID: uuid.New(), StoreID: uuid.New(), Country: "TH",
		TaxID: "1234567890123", QueueReason: "rd_manual",
	})
	require.NoError(t, err)

	require.NoError(t, repo.Resolve(context.Background(), entry.ID, reviewerID, true, "verified by ops"))
	// Second resolve is a no-op (filtered out by status condition).
	require.NoError(t, repo.Resolve(context.Background(), entry.ID, reviewerID, false, "should not flip"))

	var status string
	require.NoError(t, db.Raw(`SELECT status FROM sea_manual_review_queue WHERE id=?`, entry.ID).Row().Scan(&status))
	require.Equal(t, "approved", status)
}

func TestSEAQueue_FindByStore(t *testing.T) {
	db := testdb.NewDB(t, "sea_manual_review_queue")
	repo := seaqueue.New(db)
	storeID := uuid.New()

	none, err := repo.FindByStore(context.Background(), storeID)
	require.NoError(t, err)
	require.Nil(t, none)

	_, err = repo.Enqueue(context.Background(), seaqueue.Entry{
		TenantID: uuid.New(), StoreID: storeID, Country: "PH",
		TaxID: "123-456-789", QueueReason: "bir_manual",
	})
	require.NoError(t, err)

	got, err := repo.FindByStore(context.Background(), storeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "PH", got.Country)
}

func TestSEAQueue_CountThisWeek(t *testing.T) {
	db := testdb.NewDB(t, "sea_manual_review_queue")
	repo := seaqueue.New(db)
	for i := 0; i < 5; i++ {
		_, err := repo.Enqueue(context.Background(), seaqueue.Entry{
			TenantID: uuid.New(), StoreID: uuid.New(), Country: "ID",
			TaxID: "123456789012345", QueueReason: "djp_manual",
		})
		require.NoError(t, err)
	}
	count, err := repo.CountThisWeek(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, count, 5)
}
