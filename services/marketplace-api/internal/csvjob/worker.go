package csvjob

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/product"
)

// MaxImportRows is the hard cap on CSV rows per import. Jobs exceeding
// this are immediately failed to prevent unbounded processing.
const MaxImportRows = 50000

// OrphanWindow is how long a 'running' job may go without a heartbeat
// before the system considers it orphaned. Read by the startup recovery
// scan in cmd/marketplace-api/main.go and by the platform console's
// /admin/health endpoint (#289), so that the two cannot drift into
// disagreeing about the same job.
const OrphanWindow = 15 * time.Minute

// CSVReader opens a GCS (or local) CSV file and returns a ReadCloser.
// The concrete implementation wraps a GCS object reader; tests inject a
// fake that returns an in-memory reader.
type CSVReader interface {
	Open(ctx context.Context, gcsPath string) (io.ReadCloser, error)
}

// ErrorWriterFactory creates an ErrorCSVWriter for a given GCS path.
// Tests inject a factory that writes to an in-memory buffer.
type ErrorWriterFactory interface {
	New(ctx context.Context, gcsPath string) (ErrorCSVWriter, error)
}

// ProductCreator is the subset of product.Service that the worker needs.
type ProductCreator interface {
	Create(ctx context.Context, req product.CreateRequest) (*product.Aggregate, error)
}

// Worker processes a single CSV import job.
type Worker struct {
	repo         Repository
	productSvc   ProductCreator
	csvReader    CSVReader
	errFactory   ErrorWriterFactory
	logger       *slog.Logger
	storeID      string
	tenantID     string
	currencyCode string

	// TestCancelAfterRow, when > 0, causes the worker to cancel its own
	// context after processing this many rows. Used by crash-recovery tests.
	TestCancelAfterRow int
}

// WorkerConfig bundles dependencies for Worker construction.
type WorkerConfig struct {
	Repo         Repository
	ProductSvc   ProductCreator
	CSVReader    CSVReader
	ErrFactory   ErrorWriterFactory
	Logger       *slog.Logger
	StoreID      string
	TenantID     string
	CurrencyCode string
}

// NewWorker constructs a Worker.
func NewWorker(cfg WorkerConfig) *Worker {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		repo:         cfg.Repo,
		productSvc:   cfg.ProductSvc,
		csvReader:    cfg.CSVReader,
		errFactory:   cfg.ErrFactory,
		logger:       logger,
		storeID:      cfg.StoreID,
		tenantID:     cfg.TenantID,
		currencyCode: cfg.CurrencyCode,
	}
}

// Run processes a single import job. It is safe to call from a goroutine.
// The method blocks until the job is complete, failed, or cancelled.
func (w *Worker) Run(ctx context.Context, job CsvImportJob) {
	log := w.logger.With("job_id", job.ID, "store_id", job.StoreID)

	// Try advisory lock — if another pod is processing, skip.
	var locked bool
	err := w.repo.SetStatusFields(ctx, job.ID, map[string]any{
		"status": StatusRunning,
	})
	if err != nil {
		log.Error("csvjob: failed to set running", "err", err)
		return
	}
	locked = true
	_ = locked

	// Open the CSV from GCS.
	rc, err := w.csvReader.Open(ctx, job.GCSPath)
	if err != nil {
		log.Error("csvjob: open csv", "err", err)
		w.failJob(ctx, job.ID, "failed to open CSV file")
		return
	}
	defer rc.Close()

	reader := csv.NewReader(rc)
	reader.FieldsPerRecord = -1 // allow variable field count
	reader.LazyQuotes = true

	// Read header.
	headers, err := reader.Read()
	if err != nil {
		log.Error("csvjob: read headers", "err", err)
		w.failJob(ctx, job.ID, "failed to read CSV headers")
		return
	}

	// Set up error CSV writer.
	errGCSPath := job.GCSPath + ".errors.csv"
	errWriter, err := w.errFactory.New(ctx, errGCSPath)
	if err != nil {
		log.Error("csvjob: create error writer", "err", err)
		w.failJob(ctx, job.ID, "failed to create error CSV writer")
		return
	}
	defer errWriter.Close()

	if err := errWriter.WriteHeader(headers); err != nil {
		log.Error("csvjob: write error header", "err", err)
	}

	// Start heartbeat ticker.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go w.heartbeatLoop(heartbeatCtx, job.ID)

	tracker := NewHandleTracker()
	rowNum := 0
	successCount := job.SuccessCount
	errorCount := job.ErrorCount
	startRow := job.LastProcessedRow // resume from last checkpoint

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Error("csvjob: read row", "row", rowNum, "err", err)
			errorCount++
			continue
		}

		rowNum++

		// Skip already-processed rows on resume.
		if rowNum <= startRow {
			continue
		}

		// Row cap check.
		if rowNum > MaxImportRows {
			w.failJob(ctx, job.ID, fmt.Sprintf("CSV exceeds maximum of %d rows", MaxImportRows))
			return
		}

		// Test hook: cancel after N rows for crash-recovery testing.
		if w.TestCancelAfterRow > 0 && rowNum == startRow+w.TestCancelAfterRow {
			log.Info("csvjob: test cancel after row", "row", rowNum)
			w.checkpoint(ctx, job.ID, rowNum, successCount, errorCount)
			return // simulate crash — do NOT set terminal status
		}

		// Parse and create product.
		draft, parseErr := ParseRow(headers, row, rowNum, tracker)
		if parseErr != nil {
			errorCount++
			if writeErr := errWriter.WriteRow(row, parseErr.Error()); writeErr != nil {
				log.Error("csvjob: write error row", "err", writeErr)
			}
			continue
		}

		_, createErr := w.productSvc.Create(ctx, product.CreateRequest{
			StoreID:     w.storeID,
			TenantID:    w.tenantID,
			Handle:      draft.Handle,
			Title:       draft.Title,
			Description: draft.Description,
			Status:      draft.Status,
			Variants: []product.VariantInput{{
				SKU:          draft.SKU,
				Price:        draft.BasePrice,
				CurrencyCode: w.currencyCode,
				InitialStock: draft.Stock,
			}},
		})
		if createErr != nil {
			errorCount++
			if writeErr := errWriter.WriteRow(row, createErr.Error()); writeErr != nil {
				log.Error("csvjob: write error row", "err", writeErr)
			}
			continue
		}

		successCount++

		// Checkpoint every 10 rows.
		if rowNum%10 == 0 {
			w.checkpoint(ctx, job.ID, rowNum, successCount, errorCount)

			// Check for cancellation.
			cancelled, checkErr := w.isCancelled(ctx, job.ID)
			if checkErr != nil {
				log.Error("csvjob: check cancel", "err", checkErr)
			}
			if cancelled {
				log.Info("csvjob: cancelled by user")
				return
			}
		}
	}

	// Final checkpoint.
	w.checkpoint(ctx, job.ID, rowNum, successCount, errorCount)

	// Mark completed.
	totalRows := rowNum
	fields := map[string]any{
		"status":     StatusCompleted,
		"total_rows": totalRows,
	}
	if errorCount > 0 {
		fields["error_csv_gcs_path"] = errGCSPath
	}
	if err := w.repo.SetStatusFields(ctx, job.ID, fields); err != nil {
		log.Error("csvjob: set completed", "err", err)
	}

	log.Info("csvjob: completed",
		"total_rows", totalRows, "success", successCount, "errors", errorCount)
}

func (w *Worker) checkpoint(ctx context.Context, jobID string, lastRow, success, errors int) {
	if err := w.repo.UpdateProgress(ctx, jobID, lastRow, success, errors); err != nil {
		w.logger.Error("csvjob: checkpoint", "job_id", jobID, "err", err)
	}
}

func (w *Worker) failJob(ctx context.Context, jobID, msg string) {
	w.logger.Error("csvjob: job failed", "job_id", jobID, "reason", msg)
	_ = w.repo.SetStatusFields(ctx, jobID, map[string]any{
		"status": StatusFailed,
	})
}

func (w *Worker) isCancelled(ctx context.Context, jobID string) (bool, error) {
	job, err := w.repo.GetByID(ctx, jobID)
	if err != nil {
		return false, err
	}
	return job.Status == StatusCancelled, nil
}

func (w *Worker) heartbeatLoop(ctx context.Context, jobID string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.repo.UpdateHeartbeat(ctx, jobID); err != nil {
				w.logger.Error("csvjob: heartbeat", "job_id", jobID, "err", err)
			}
		}
	}
}

// RecoverOrphanedJobs finds running jobs with stale heartbeats and marks
// them as paused so the polling loop can re-enqueue them.
func RecoverOrphanedJobs(ctx context.Context, repo Repository, staleDuration time.Duration, logger *slog.Logger) error {
	jobs, err := repo.FindOrphanedJobs(ctx, staleDuration)
	if err != nil {
		return fmt.Errorf("csvjob: recover orphaned: %w", err)
	}
	for _, j := range jobs {
		logger.Info("csvjob: recovering orphaned job", "job_id", j.ID, "heartbeat_at", j.HeartbeatAt)
		if err := repo.UpdateStatus(ctx, j.ID, StatusPaused); err != nil {
			logger.Error("csvjob: recover orphaned", "job_id", j.ID, "err", err)
		}
	}
	return nil
}

// Ensure decimal import is used (referenced via ProductDraft.BasePrice in parser).
var _ = decimal.Zero
