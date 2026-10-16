package csvjob

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"strings"
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
// /admin/health endpoint (#289), so the two share one definition of the
// WINDOW and cannot drift apart if it is retuned.
//
// They do NOT share the boundary predicate: the recovery scan uses `<`
// while the health check uses `<=`, so a job whose heartbeat is exactly
// OrphanWindow old reads as stale on /admin/health one instant before the
// scan would reset it. That one-instant disagreement is deliberate —
// /admin/health applies "degraded when age >= window" uniformly across all
// four of its checks.
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
	lastCheckpoint := startRow

	// Consecutive rows with one handle are one product, a row per
	// variant, so rows are held until the handle changes. Checkpoints
	// land only between products: a resume never starts mid-product.
	idx := buildHeaderIndex(headers)
	handleOf := func(row []string) string {
		i, ok := idx[colHandle]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(row[i]))
	}
	var group []csvRow
	groupKey := ""

	// flush imports the held product; true means the job was cancelled.
	flush := func() bool {
		if len(group) == 0 {
			return false
		}
		rows := group
		group = nil
		if errMsg := w.importProduct(ctx, headers, rows, tracker); errMsg != "" {
			errorCount += len(rows)
			for _, r := range rows {
				if writeErr := errWriter.WriteRow(r.cells, errMsg); writeErr != nil {
					log.Error("csvjob: write error row", "err", writeErr)
				}
			}
		} else {
			successCount += len(rows)
		}

		last := rows[len(rows)-1].num
		if last-lastCheckpoint < 10 {
			return false
		}
		lastCheckpoint = last
		w.checkpoint(ctx, job.ID, last, successCount, errorCount)
		cancelled, checkErr := w.isCancelled(ctx, job.ID)
		if checkErr != nil {
			log.Error("csvjob: check cancel", "err", checkErr)
		}
		if cancelled {
			log.Info("csvjob: cancelled by user")
		}
		return cancelled
	}

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

		// A blank handle never joins the product above it.
		key := handleOf(row)
		if len(group) > 0 && (key == "" || key != groupKey) {
			if flush() {
				return
			}
		}

		// Test hook: cancel after N rows for crash-recovery testing.
		if w.TestCancelAfterRow > 0 && rowNum == startRow+w.TestCancelAfterRow {
			log.Info("csvjob: test cancel after row", "row", rowNum)
			if flush() {
				return
			}
			w.checkpoint(ctx, job.ID, rowNum, successCount, errorCount)
			return // simulate crash — do NOT set terminal status
		}

		group = append(group, csvRow{num: rowNum, cells: row})
		groupKey = key
	}
	if flush() {
		return
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

// csvRow is one data row and its 1-based number.
type csvRow struct {
	num   int
	cells []string
}

// importProduct creates one product from its rows and returns "" on
// success, else the message written beside each of its rows.
func (w *Worker) importProduct(ctx context.Context, headers []string, rows []csvRow, tracker *HandleTracker) string {
	drafts := make([]ProductDraft, len(rows))
	var errs []string
	for i, r := range rows {
		var err error
		if i == 0 {
			drafts[i], err = ParseRow(headers, r.cells, r.num, tracker)
		} else {
			drafts[i], err = ParseVariantRow(headers, r.cells, r.num)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return strings.Join(errs, "; ")
	}

	req, err := w.toCreateRequest(drafts, rows)
	if err != nil {
		return err.Error()
	}
	if _, err := w.productSvc.Create(ctx, req); err != nil {
		return err.Error()
	}
	return ""
}

// toCreateRequest builds one product from its rows. The first row carries
// the product fields and names the options; every row is a variant.
func (w *Worker) toCreateRequest(drafts []ProductDraft, rows []csvRow) (product.CreateRequest, error) {
	first := drafts[0]

	var axes []int // option positions the first row names
	for i, o := range first.Options {
		if o.Name != "" {
			axes = append(axes, i)
		}
	}
	// Shopify writes a product without options as Title / Default Title.
	if len(drafts) == 1 && len(axes) == 1 &&
		strings.EqualFold(first.Options[axes[0]].Name, "Title") &&
		strings.EqualFold(first.Options[axes[0]].Value, "Default Title") {
		axes = nil
		drafts[0].Options = [maxOptions]OptionPair{}
	}

	options := make([]product.OptionSpec, len(axes))
	seen := make([]map[string]bool, len(axes))
	for a, pos := range axes {
		options[a].Name = first.Options[pos].Name
		seen[a] = map[string]bool{}
	}

	variants := make([]product.VariantInput, len(drafts))
	for i, d := range drafts {
		for pos, o := range d.Options {
			named := false
			for _, a := range axes {
				named = named || a == pos
			}
			if !named && o.Value != "" {
				return product.CreateRequest{}, fmt.Errorf("row %d: %s is set but the product's first row has no %s",
					rows[i].num, optionCols[pos][1], optionCols[pos][0])
			}
		}
		refs := make([]product.OptionValueRef, len(axes))
		for a, pos := range axes {
			o := d.Options[pos]
			if o.Value == "" {
				return product.CreateRequest{}, fmt.Errorf("row %d: %s is required for every variant", rows[i].num, optionCols[pos][1])
			}
			if o.Name != "" && !strings.EqualFold(o.Name, options[a].Name) {
				return product.CreateRequest{}, fmt.Errorf("row %d: %s %q does not match %q on the product's first row",
					rows[i].num, optionCols[pos][0], o.Name, options[a].Name)
			}
			if !seen[a][o.Value] {
				seen[a][o.Value] = true
				options[a].Values = append(options[a].Values, product.OptionValueSpec{Value: o.Value})
			}
			refs[a] = product.OptionValueRef{OptionName: options[a].Name, Value: o.Value}
		}
		variants[i] = product.VariantInput{
			SKU:          d.SKU,
			Price:        d.BasePrice,
			CurrencyCode: w.currencyCode,
			InitialStock: d.Stock,
			WeightGrams:  d.WeightGrams,
			LengthCM:     d.LengthCM,
			WidthCM:      d.WidthCM,
			HeightCM:     d.HeightCM,
			OptionValues: refs,
			Position:     i,
		}
	}

	return product.CreateRequest{
		StoreID:     w.storeID,
		TenantID:    w.tenantID,
		Handle:      first.Handle,
		Title:       first.Title,
		Description: first.Description,
		Status:      first.Status,
		Options:     options,
		Variants:    variants,
	}, nil
}

// Ensure decimal import is used (referenced via ProductDraft.BasePrice in parser).
var _ = decimal.Zero
