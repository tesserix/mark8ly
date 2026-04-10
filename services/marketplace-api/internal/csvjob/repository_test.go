//go:build integration

package csvjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedStoreForCSV(t *testing.T, db *gorm.DB) string {
	t.Helper()
	storeID := uuid.NewString()
	tenantID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "csv-" + storeID[:8],
		Name:         "CSV Test Store",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
	}
	require.NoError(t, db.Create(s).Error)
	return storeID
}

func TestRepository_CreateAndGet(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := csvjob.NewRepository(tx)
	ctx := context.Background()

	storeID := seedStoreForCSV(t, tx)
	job := &csvjob.CsvImportJob{
		ID:          uuid.NewString(),
		StoreID:     storeID,
		UserID:      "user-1",
		GCSPath:     "csv-imports/test.csv",
		ContentHash: "abc123",
		Status:      csvjob.StatusQueued,
	}

	err := repo.Create(ctx, job)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.ID, got.ID)
	require.Equal(t, storeID, got.StoreID)
	require.Equal(t, "user-1", got.UserID)
	require.Equal(t, csvjob.StatusQueued, got.Status)
	require.Equal(t, 0, got.LastProcessedRow)
}

func TestRepository_FindOrphanedJobs(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := csvjob.NewRepository(tx)
	ctx := context.Background()

	storeID := seedStoreForCSV(t, tx)

	// Create a job with stale heartbeat (running, heartbeat 30 min ago).
	staleTime := time.Now().Add(-30 * time.Minute)
	staleJob := &csvjob.CsvImportJob{
		ID:          uuid.NewString(),
		StoreID:     storeID,
		UserID:      "user-1",
		GCSPath:     "csv-imports/stale.csv",
		ContentHash: "stale-hash",
		Status:      csvjob.StatusRunning,
		HeartbeatAt: &staleTime,
	}
	require.NoError(t, repo.Create(ctx, staleJob))

	// Create a fresh running job (heartbeat 1 sec ago).
	freshTime := time.Now().Add(-1 * time.Second)
	freshJob := &csvjob.CsvImportJob{
		ID:          uuid.NewString(),
		StoreID:     storeID,
		UserID:      "user-1",
		GCSPath:     "csv-imports/fresh.csv",
		ContentHash: "fresh-hash",
		Status:      csvjob.StatusRunning,
		HeartbeatAt: &freshTime,
	}
	require.NoError(t, repo.Create(ctx, freshJob))

	// FindOrphanedJobs with 10-minute staleness threshold.
	orphaned, err := repo.FindOrphanedJobs(ctx, 10*time.Minute)
	require.NoError(t, err)
	require.Len(t, orphaned, 1)
	require.Equal(t, staleJob.ID, orphaned[0].ID)
}

func TestRepository_FindByContentHash(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := csvjob.NewRepository(tx)
	ctx := context.Background()

	storeID := seedStoreForCSV(t, tx)
	hash := "dedupe-hash-" + uuid.NewString()[:8]

	// No active job yet.
	got, err := repo.FindByContentHash(ctx, storeID, hash)
	require.NoError(t, err)
	require.Nil(t, got)

	// Create an active job.
	job := &csvjob.CsvImportJob{
		ID:          uuid.NewString(),
		StoreID:     storeID,
		UserID:      "user-1",
		GCSPath:     "csv-imports/dedupe.csv",
		ContentHash: hash,
		Status:      csvjob.StatusQueued,
	}
	require.NoError(t, repo.Create(ctx, job))

	got, err = repo.FindByContentHash(ctx, storeID, hash)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, job.ID, got.ID)

	// Complete the job — should no longer match.
	require.NoError(t, repo.UpdateStatus(ctx, job.ID, csvjob.StatusCompleted))
	got, err = repo.FindByContentHash(ctx, storeID, hash)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestRepository_ListByStore(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := csvjob.NewRepository(tx)
	ctx := context.Background()

	storeID := seedStoreForCSV(t, tx)

	for i := 0; i < 3; i++ {
		job := &csvjob.CsvImportJob{
			ID:          uuid.NewString(),
			StoreID:     storeID,
			UserID:      "user-1",
			GCSPath:     "csv-imports/list.csv",
			ContentHash: uuid.NewString(),
			Status:      csvjob.StatusCompleted,
		}
		require.NoError(t, repo.Create(ctx, job))
	}

	jobs, total, err := repo.ListByStore(ctx, storeID, 1, 2)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, jobs, 2)
}
