package csvjob_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// fakeRepo is an in-memory csvjob.Repository for unit tests.
type fakeRepo struct {
	jobs map[string]*csvjob.CsvImportJob
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{jobs: make(map[string]*csvjob.CsvImportJob)}
}

func (f *fakeRepo) Create(_ context.Context, job *csvjob.CsvImportJob) error {
	f.jobs[job.ID] = job
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (*csvjob.CsvImportJob, error) {
	j, ok := f.jobs[id]
	if !ok {
		return nil, apperrors.NotFound("csv_import_job")
	}
	return j, nil
}

func (f *fakeRepo) ListByStore(_ context.Context, storeID string, page, pageSize int) ([]csvjob.CsvImportJob, int64, error) {
	var out []csvjob.CsvImportJob
	for _, j := range f.jobs {
		if j.StoreID == storeID {
			out = append(out, *j)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepo) UpdateStatus(_ context.Context, id, status string) error {
	j, ok := f.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	j.Status = status
	return nil
}

func (f *fakeRepo) UpdateProgress(_ context.Context, id string, lastRow, successCount, errorCount int) error {
	j, ok := f.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	j.LastProcessedRow = lastRow
	j.SuccessCount = successCount
	j.ErrorCount = errorCount
	return nil
}

func (f *fakeRepo) UpdateHeartbeat(_ context.Context, id string) error {
	j, ok := f.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	now := time.Now()
	j.HeartbeatAt = &now
	return nil
}

func (f *fakeRepo) FindOrphanedJobs(_ context.Context, staleDuration time.Duration) ([]csvjob.CsvImportJob, error) {
	cutoff := time.Now().Add(-staleDuration)
	var out []csvjob.CsvImportJob
	for _, j := range f.jobs {
		if j.Status == csvjob.StatusRunning && (j.HeartbeatAt == nil || j.HeartbeatAt.Before(cutoff)) {
			out = append(out, *j)
		}
	}
	return out, nil
}

func (f *fakeRepo) FindByContentHash(_ context.Context, storeID, contentHash string) (*csvjob.CsvImportJob, error) {
	for _, j := range f.jobs {
		if j.StoreID == storeID && j.ContentHash == contentHash &&
			(j.Status == csvjob.StatusQueued || j.Status == csvjob.StatusRunning || j.Status == csvjob.StatusPaused) {
			return j, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) SetStatusFields(_ context.Context, id string, fields map[string]any) error {
	j, ok := f.jobs[id]
	if !ok {
		return apperrors.NotFound("csv_import_job")
	}
	if s, ok := fields["status"]; ok {
		j.Status = s.(string)
	}
	return nil
}

func TestService_SubmitCreatesNew(t *testing.T) {
	repo := newFakeRepo()
	svc := csvjob.NewService(repo, nil)
	ctx := context.Background()

	body := bytes.NewReader([]byte("title,handle\nShirt,shirt\n"))
	result, err := svc.Submit(ctx, "store-1", "user-1", "gcs/path.csv", body)
	require.NoError(t, err)
	require.False(t, result.Deduplicated)
	require.Equal(t, csvjob.StatusQueued, result.Job.Status)
	require.Equal(t, "store-1", result.Job.StoreID)
}

func TestService_SubmitDeduplicates(t *testing.T) {
	repo := newFakeRepo()
	svc := csvjob.NewService(repo, nil)
	ctx := context.Background()

	csvData := []byte("title,handle\nShirt,shirt\n")
	h := sha256.Sum256(csvData)
	hash := hex.EncodeToString(h[:])

	// First submit.
	result1, err := svc.Submit(ctx, "store-1", "user-1", "gcs/path.csv", bytes.NewReader(csvData))
	require.NoError(t, err)
	require.False(t, result1.Deduplicated)
	require.Equal(t, hash, result1.Job.ContentHash)

	// Second submit with identical content.
	result2, err := svc.Submit(ctx, "store-1", "user-1", "gcs/path2.csv", bytes.NewReader(csvData))
	require.NoError(t, err)
	require.True(t, result2.Deduplicated)
	require.Equal(t, result1.Job.ID, result2.Job.ID)
}

func TestService_CancelSetsStatus(t *testing.T) {
	repo := newFakeRepo()
	svc := csvjob.NewService(repo, nil)
	ctx := context.Background()

	body := bytes.NewReader([]byte("data"))
	result, err := svc.Submit(ctx, "store-1", "user-1", "gcs/cancel.csv", body)
	require.NoError(t, err)

	err = svc.Cancel(ctx, result.Job.ID)
	require.NoError(t, err)

	job, err := svc.GetStatus(ctx, result.Job.ID)
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusCancelled, job.Status)
}

func TestService_CancelTerminalFails(t *testing.T) {
	repo := newFakeRepo()
	svc := csvjob.NewService(repo, nil)
	ctx := context.Background()

	body := bytes.NewReader([]byte("data"))
	result, err := svc.Submit(ctx, "store-1", "user-1", "gcs/done.csv", body)
	require.NoError(t, err)

	// Force to completed.
	require.NoError(t, repo.UpdateStatus(ctx, result.Job.ID, csvjob.StatusCompleted))

	err = svc.Cancel(ctx, result.Job.ID)
	require.Error(t, err)
	var ae *apperrors.Error
	require.True(t, errors.As(err, &ae))
	require.Equal(t, apperrors.CodeInvalidTransition, ae.Code)
}

func TestService_ResumeFromPaused(t *testing.T) {
	repo := newFakeRepo()
	svc := csvjob.NewService(repo, nil)
	ctx := context.Background()

	body := bytes.NewReader([]byte("data"))
	result, err := svc.Submit(ctx, "store-1", "user-1", "gcs/paused.csv", body)
	require.NoError(t, err)

	// Pause.
	require.NoError(t, repo.UpdateStatus(ctx, result.Job.ID, csvjob.StatusPaused))

	err = svc.Resume(ctx, result.Job.ID)
	require.NoError(t, err)

	job, err := svc.GetStatus(ctx, result.Job.ID)
	require.NoError(t, err)
	require.Equal(t, csvjob.StatusQueued, job.Status)
}

func TestService_ResumeFromNonPausedFails(t *testing.T) {
	repo := newFakeRepo()
	svc := csvjob.NewService(repo, nil)
	ctx := context.Background()

	body := bytes.NewReader([]byte("data"))
	result, err := svc.Submit(ctx, "store-1", "user-1", "gcs/running.csv", body)
	require.NoError(t, err)

	// Job is queued, not paused.
	err = svc.Resume(ctx, result.Job.ID)
	require.Error(t, err)
}
