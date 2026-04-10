package csvjob_test

import (
	"context"
	"testing"
	"log/slog"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

// TestCrashRecovery_OrphanedJobPausedAndResumed simulates a worker crash:
// 1. Create a job with status='running' and stale heartbeat (30 min ago).
// 2. Run the recovery scan → assert job moves to 'paused'.
// 3. Resume the job → assert it goes back to 'queued'.
// 4. Run the worker with a fresh CSV → assert it resumes from
//    last_processed_row and produces no duplicate products.
func TestCrashRecovery_OrphanedJobPausedAndResumed(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()

	staleTime := time.Now().Add(-30 * time.Minute)
	job := &csvjob.CsvImportJob{
		ID:               "crash-job",
		StoreID:          "store-1",
		UserID:           "user-1",
		GCSPath:          "crash.csv",
		ContentHash:      "crash-hash",
		Status:           csvjob.StatusRunning,
		HeartbeatAt:      &staleTime,
		LastProcessedRow: 5,
		SuccessCount:     5,
		ErrorCount:       0,
	}
	require.NoError(t, repo.Create(ctx, job))

	// Step 2: Recovery scan — marks orphaned jobs as paused.
	err := csvjob.RecoverOrphanedJobs(ctx, repo, 10*time.Minute, slog.Default())
	require.NoError(t, err)

	recovered, err := repo.GetByID(ctx, "crash-job")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusPaused, recovered.Status)

	// Step 3: Resume.
	svc := csvjob.NewService(repo, nil)
	require.NoError(t, svc.Resume(ctx, "crash-job"))

	resumed, err := repo.GetByID(ctx, "crash-job")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusQueued, resumed.Status)

	// Step 4: Run worker — CSV has 10 rows total, 5 already processed.
	csvContent := "title,handle,base_price,sku,stock\n"
	for i := 1; i <= 10; i++ {
		csvContent += "Product,handle-" + itoa(i) + ",10.00,SKU-" + itoa(i) + ",5\n"
	}

	creator := &fakeProductCreator{}
	errFactory := newFakeErrorWriterFactory()
	w := csvjob.NewWorker(csvjob.WorkerConfig{
		Repo:         repo,
		ProductSvc:   creator,
		CSVReader:    &fakeCSVReader{content: csvContent},
		ErrFactory:   errFactory,
		StoreID:      "store-1",
		TenantID:     "tenant-1",
		CurrencyCode: "USD",
	})

	w.Run(ctx, *resumed)

	final, err := repo.GetByID(ctx, "crash-job")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusCompleted, final.Status)
	require.Equal(t, 10, final.LastProcessedRow)
	// 5 from before crash + 5 new = 10 total.
	require.Equal(t, 10, final.SuccessCount)
	require.Equal(t, 0, final.ErrorCount)

	// Only 5 new products created (rows 6-10), no duplicates.
	require.Len(t, creator.calls, 5)
}

// TestCrashRecovery_TestCancelAfterRow verifies the TestCancelAfterRow
// hook: worker exits after N rows, recovery scan pauses it, resume works.
func TestCrashRecovery_TestCancelAfterRow(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()

	csvContent := "title,handle,base_price,sku,stock\n"
	for i := 1; i <= 20; i++ {
		csvContent += "Product,handle-" + itoa(i) + ",10.00,SKU-" + itoa(i) + ",5\n"
	}

	job := &csvjob.CsvImportJob{
		ID:          "cancel-crash-job",
		StoreID:     "store-1",
		UserID:      "user-1",
		GCSPath:     "cancel-crash.csv",
		ContentHash: "cancel-crash-hash",
		Status:      csvjob.StatusQueued,
	}
	require.NoError(t, repo.Create(ctx, job))

	creator := &fakeProductCreator{}
	errFactory := newFakeErrorWriterFactory()
	w := csvjob.NewWorker(csvjob.WorkerConfig{
		Repo:         repo,
		ProductSvc:   creator,
		CSVReader:    &fakeCSVReader{content: csvContent},
		ErrFactory:   errFactory,
		StoreID:      "store-1",
		TenantID:     "tenant-1",
		CurrencyCode: "USD",
	})
	w.TestCancelAfterRow = 7

	// Run 1: processes 6 rows, cancels at row 7.
	w.Run(ctx, *job)

	mid, err := repo.GetByID(ctx, "cancel-crash-job")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusRunning, mid.Status) // simulated crash
	require.Equal(t, 7, mid.LastProcessedRow)
	require.Equal(t, 6, mid.SuccessCount)

	// Recovery scan pauses it.
	// Set heartbeat to stale so FindOrphanedJobs picks it up.
	stale := time.Now().Add(-20 * time.Minute)
	mid.HeartbeatAt = &stale
	// Update directly in fake repo.
	require.NoError(t, repo.SetStatusFields(ctx, mid.ID, map[string]any{
		"status": csvjob.StatusRunning,
	}))
	repo.jobs[mid.ID].HeartbeatAt = &stale

	require.NoError(t, csvjob.RecoverOrphanedJobs(ctx, repo, 10*time.Minute, slog.Default()))

	paused, err := repo.GetByID(ctx, "cancel-crash-job")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusPaused, paused.Status)

	// Resume and run to completion.
	require.NoError(t, csvjob.NewService(repo, nil).Resume(ctx, "cancel-crash-job"))

	resumed, _ := repo.GetByID(ctx, "cancel-crash-job")
	w2 := csvjob.NewWorker(csvjob.WorkerConfig{
		Repo:         repo,
		ProductSvc:   creator,
		CSVReader:    &fakeCSVReader{content: csvContent},
		ErrFactory:   errFactory,
		StoreID:      "store-1",
		TenantID:     "tenant-1",
		CurrencyCode: "USD",
	})
	w2.Run(ctx, *resumed)

	final, err := repo.GetByID(ctx, "cancel-crash-job")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusCompleted, final.Status)
	// 6 from run 1 (rows 1-6) + 13 from run 2 (rows 8-20) = 19.
	// Row 7 was checkpointed but not processed (cancel fires at start of row).
	require.Equal(t, 19, final.SuccessCount)
	require.Equal(t, 0, final.ErrorCount)
	// 6 + 13 = 19 product.Create calls total.
	require.Len(t, creator.calls, 19)
}
