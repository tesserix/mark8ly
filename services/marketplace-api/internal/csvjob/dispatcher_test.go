package csvjob_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

type fakeStoreLookup struct {
	tenantID string
	currency string
	err      error
}

func (f *fakeStoreLookup) TenantAndCurrency(_ context.Context, _ string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.tenantID, f.currency, nil
}

func queuedJob(id, storeID string) *csvjob.CsvImportJob {
	return &csvjob.CsvImportJob{
		ID:      id,
		StoreID: storeID,
		Status:  csvjob.StatusQueued,
		GCSPath: "csv-imports/" + storeID + "/" + id + ".csv",
	}
}

func newTestDispatcher(repo csvjob.Repository, stores csvjob.StoreLookup, creator csvjob.ProductCreator, csv string) *csvjob.Dispatcher {
	return csvjob.NewDispatcher(csvjob.DispatcherConfig{
		Repo:       repo,
		Stores:     stores,
		Products:   creator,
		Reader:     &fakeCSVReader{content: csv},
		ErrFactory: newFakeErrorWriterFactory(),
	})
}

// The defect this closes: a queued job stayed queued forever because
// nothing ever called NewWorker (#897). Products created is the only
// evidence that matters here — a status change alone could come from a
// dispatcher that claimed the job and then did nothing with it.
func TestDispatcher_RunsQueuedJobAndCreatesProducts(t *testing.T) {
	repo := newFakeRepo()
	require.NoError(t, repo.Create(context.Background(), queuedJob("job-1", "store-1")))

	creator := &fakeProductCreator{}
	d := newTestDispatcher(repo, &fakeStoreLookup{tenantID: "tenant-1", currency: "AUD"}, creator,
		"title,handle,base_price,sku,stock\nShirt,shirt-1,10.00,S1,5\n")

	runUntilDrained(t, d, repo)

	require.Len(t, creator.calls, 1, "the queued CSV row should have produced a product")
	job, err := repo.GetByID(context.Background(), "job-1")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusCompleted, job.Status)
}

// Two admin replicas both poll. Whichever loses the claim must not run
// the job a second time, or the merchant's catalog gets duplicates.
func TestDispatcher_SkipsJobClaimedByAnotherReplica(t *testing.T) {
	repo := newFakeRepo()
	require.NoError(t, repo.Create(context.Background(), queuedJob("job-1", "store-1")))

	// Simulate the other replica winning the race.
	claimed, err := repo.ClaimJob(context.Background(), "job-1")
	require.NoError(t, err)
	require.True(t, claimed)

	creator := &fakeProductCreator{}
	d := newTestDispatcher(repo, &fakeStoreLookup{tenantID: "tenant-1", currency: "AUD"}, creator,
		"title,handle,base_price,sku,stock\nShirt,shirt-1,10.00,S1,5\n")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	<-d.Start(ctx, 10*time.Millisecond)

	require.Empty(t, creator.calls, "a job already claimed elsewhere must not be run again")
}

// A store that cannot be resolved has no tenant and no currency, so the
// job cannot run. Failing it is the honest outcome; leaving it running
// with no heartbeat would recreate the silent stall.
func TestDispatcher_FailsJobWhenStoreCannotBeResolved(t *testing.T) {
	repo := newFakeRepo()
	require.NoError(t, repo.Create(context.Background(), queuedJob("job-1", "store-gone")))

	creator := &fakeProductCreator{}
	d := newTestDispatcher(repo, &fakeStoreLookup{err: errors.New("no such store")}, creator,
		"title,handle,base_price,sku,stock\nShirt,shirt-1,10.00,S1,5\n")

	runUntilDrained(t, d, repo)

	require.Empty(t, creator.calls)
	job, err := repo.GetByID(context.Background(), "job-1")
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusFailed, job.Status)
}

// runUntilDrained ticks the dispatcher until the queue empties, then stops
// it. Waiting on the outcome rather than sleeping a fixed span keeps this
// from being the flaky kind of test that only passes on a fast machine.
func runUntilDrained(t *testing.T, d *csvjob.Dispatcher, repo csvjob.Repository) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := d.Start(ctx, 5*time.Millisecond)
	defer func() {
		cancel()
		<-done
	}()
	require.Eventually(t, func() bool {
		jobs, err := repo.FindQueuedJobs(context.Background(), 10)
		return err == nil && len(jobs) == 0
	}, 2*time.Second, 5*time.Millisecond, "the dispatcher never drained the queue")
}
