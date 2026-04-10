package csvjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service orchestrates CSV import job submission and lifecycle.
type Service struct {
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a Service from the given repository and logger.
func NewService(repo Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger}
}

// SubmitResult is the return value of Submit. Deduplicated indicates the
// job was already active for the same content hash.
type SubmitResult struct {
	Job          CsvImportJob
	Deduplicated bool
}

// Submit creates a new CSV import job or returns an existing active job if
// the same content hash is already queued/running/paused for this store.
func (s *Service) Submit(ctx context.Context, storeID, userID, gcsPath string, csvBody io.Reader) (*SubmitResult, error) {
	hash, err := computeSHA256(csvBody)
	if err != nil {
		return nil, fmt.Errorf("csvjob: compute hash: %w", err)
	}

	existing, err := s.repo.FindByContentHash(ctx, storeID, hash)
	if err != nil {
		return nil, fmt.Errorf("csvjob: check dedupe: %w", err)
	}
	if existing != nil {
		s.logger.Info("csvjob: deduplicated import",
			"store_id", storeID, "existing_job_id", existing.ID, "content_hash", hash)
		return &SubmitResult{Job: *existing, Deduplicated: true}, nil
	}

	job := &CsvImportJob{
		ID:          uuid.NewString(),
		StoreID:     storeID,
		UserID:      userID,
		GCSPath:     gcsPath,
		ContentHash: hash,
		Status:      StatusQueued,
	}
	if err := s.repo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("csvjob: create job: %w", err)
	}

	s.logger.Info("csvjob: submitted new import",
		"store_id", storeID, "job_id", job.ID, "content_hash", hash)
	return &SubmitResult{Job: *job, Deduplicated: false}, nil
}

// SubmitWithHash is like Submit but accepts a pre-computed content hash
// and skips reading the CSV body. Used when the hash was computed during
// the upload phase.
func (s *Service) SubmitWithHash(ctx context.Context, storeID, userID, gcsPath, contentHash string) (*SubmitResult, error) {
	existing, err := s.repo.FindByContentHash(ctx, storeID, contentHash)
	if err != nil {
		return nil, fmt.Errorf("csvjob: check dedupe: %w", err)
	}
	if existing != nil {
		return &SubmitResult{Job: *existing, Deduplicated: true}, nil
	}

	job := &CsvImportJob{
		ID:          uuid.NewString(),
		StoreID:     storeID,
		UserID:      userID,
		GCSPath:     gcsPath,
		ContentHash: contentHash,
		Status:      StatusQueued,
	}
	if err := s.repo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("csvjob: create job: %w", err)
	}
	return &SubmitResult{Job: *job, Deduplicated: false}, nil
}

// GetStatus returns the current state of a job.
func (s *Service) GetStatus(ctx context.Context, jobID string) (*CsvImportJob, error) {
	return s.repo.GetByID(ctx, jobID)
}

// Cancel sets the job status to cancelled if it is in a cancellable state
// (queued, running, or paused).
func (s *Service) Cancel(ctx context.Context, jobID string) error {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status != StatusQueued && job.Status != StatusRunning && job.Status != StatusPaused {
		return apperrors.New(apperrors.CodeInvalidTransition,
			fmt.Sprintf("cannot cancel job in status %q", job.Status))
	}
	return s.repo.UpdateStatus(ctx, jobID, StatusCancelled)
}

// Resume re-queues a paused job so the worker picks it up again.
func (s *Service) Resume(ctx context.Context, jobID string) error {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status != StatusPaused {
		return apperrors.New(apperrors.CodeInvalidTransition,
			fmt.Sprintf("cannot resume job in status %q", job.Status))
	}
	return s.repo.UpdateStatus(ctx, jobID, StatusQueued)
}

// ListByStore returns paginated import jobs for a store.
func (s *Service) ListByStore(ctx context.Context, storeID string, page, pageSize int) ([]CsvImportJob, int64, error) {
	return s.repo.ListByStore(ctx, storeID, page, pageSize)
}

func computeSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
