package csvjob

import (
	"context"
	"log/slog"
	"time"
)

// StoreLookup resolves the tenant and currency a job's products belong to.
// The job row carries only store_id; WorkerConfig needs both of the others.
type StoreLookup interface {
	TenantAndCurrency(ctx context.Context, storeID string) (tenantID, currencyCode string, err error)
}

// Dispatcher drains the queued-job backlog.
//
// Everything below the queue already existed — Worker.Run, checkpointing,
// heartbeats, orphan recovery, the tests. What was missing was anything
// that called it: NewWorker had no production caller, and FindQueuedJobs
// existed on the repository for a consumer that was never written, so every
// upload sat at status=queued forever (#897).
//
// In-process rather than a separate binary, matching webhook.NewWorker
// which main.go already starts this way. RecoverOrphanedJobs exists for
// exactly the crash case that choice implies: a pod that dies mid-import
// leaves a stale heartbeat, and recovery re-queues it.
type Dispatcher struct {
	repo       Repository
	stores     StoreLookup
	products   ProductCreator
	reader     CSVReader
	errFactory ErrorWriterFactory
	logger     *slog.Logger
	batch      int
}

// DispatcherConfig bundles the dependencies.
type DispatcherConfig struct {
	Repo       Repository
	Stores     StoreLookup
	Products   ProductCreator
	Reader     CSVReader
	ErrFactory ErrorWriterFactory
	Logger     *slog.Logger
	// Batch caps jobs claimed per tick. Imports run up to the 50,000-row
	// limit the UI advertises, so a small batch keeps one large import from
	// monopolising the pod while other tenants wait.
	Batch int
}

// NewDispatcher constructs a Dispatcher.
func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	batch := cfg.Batch
	if batch <= 0 {
		batch = 2
	}
	return &Dispatcher{
		repo:       cfg.Repo,
		stores:     cfg.Stores,
		products:   cfg.Products,
		reader:     cfg.Reader,
		errFactory: cfg.ErrFactory,
		logger:     logger,
		batch:      batch,
	}
}

// Start polls until ctx is cancelled, returning a channel closed on exit so
// shutdown can wait for an in-flight import rather than truncating it.
func (d *Dispatcher) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				d.tick(ctx)
			}
		}
	}()
	return done
}

// tick claims and runs whatever is queued, one job at a time.
//
// Sequential on purpose: each Run streams a whole CSV and creates products,
// and the pods this shares with serve the admin API. Throughput is not the
// constraint here — an import that finishes a minute later is fine; an
// admin UI that stalls because imports ate the pod is not.
func (d *Dispatcher) tick(ctx context.Context) {
	jobs, err := d.repo.FindQueuedJobs(ctx, d.batch)
	if err != nil {
		d.logger.Error("csvjob: poll queued jobs", "err", err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		d.runOne(ctx, job)
	}
}

func (d *Dispatcher) runOne(ctx context.Context, job CsvImportJob) {
	claimed, err := d.repo.ClaimJob(ctx, job.ID)
	if err != nil {
		d.logger.Error("csvjob: claim", "job_id", job.ID, "err", err)
		return
	}
	if !claimed {
		// The other replica took it. Normal, not an error.
		return
	}

	tenantID, currency, err := d.stores.TenantAndCurrency(ctx, job.StoreID)
	if err != nil {
		// Fail the job rather than leave it running with no heartbeat: a
		// silent stall is what this whole issue was made of.
		d.logger.Error("csvjob: resolve store", "job_id", job.ID, "store_id", job.StoreID, "err", err)
		if serr := d.repo.SetStatusFields(ctx, job.ID, map[string]any{
			"status":        StatusFailed,
			"error_message": "could not resolve the store for this import",
		}); serr != nil {
			d.logger.Error("csvjob: mark failed", "job_id", job.ID, "err", serr)
		}
		return
	}

	d.logger.Info("csvjob: starting import", "job_id", job.ID, "store_id", job.StoreID)
	NewWorker(WorkerConfig{
		Repo:         d.repo,
		ProductSvc:   d.products,
		CSVReader:    d.reader,
		ErrFactory:   d.errFactory,
		Logger:       d.logger,
		StoreID:      job.StoreID,
		TenantID:     tenantID,
		CurrencyCode: currency,
	}).Run(ctx, job)
}
