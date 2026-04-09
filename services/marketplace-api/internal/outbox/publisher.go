// Package outbox: publisher goroutine. See spec §14.1 and §14.6.
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// Publisher polls outbox_events and bumps store_watermarks asynchronously.
// See spec §14.1 (watermark separation) and §14.6 (publisher semantics).
//
// Payload invariant: every outbox row in slice 1 carries a "store_id" key
// at the top level of its JSON payload. Rows without it are logged and
// marked published without a watermark bump — losing the signal is
// preferable to blocking the publisher on a producer bug.
type Publisher struct {
	repo     Repository
	db       *gorm.DB
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

// Config configures a Publisher.
type Config struct {
	Repo      Repository
	DB        *gorm.DB
	Logger    *slog.Logger
	Interval  time.Duration // default 2s
	BatchSize int           // default 100
}

// New constructs a Publisher.
func New(cfg Config) *Publisher {
	if cfg.Interval == 0 {
		cfg.Interval = 2 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	return &Publisher{
		repo:     cfg.Repo,
		db:       cfg.DB,
		logger:   cfg.Logger,
		interval: cfg.Interval,
		batch:    cfg.BatchSize,
	}
}

// Start runs the publisher loop until ctx is cancelled. Returns a channel
// that closes when the loop exits. Callers cancel ctx to stop and wait
// on the channel for shutdown.
func (p *Publisher) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(p.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := p.Tick(ctx); err != nil && p.logger != nil {
					p.logger.Error("outbox publisher tick failed", "err", err)
				}
			}
		}
	}()
	return done
}

// Tick processes a single batch. Exposed for tests that want to drive
// without sleeping.
func (p *Publisher) Tick(ctx context.Context) (int, error) {
	return p.repo.ProcessBatch(ctx, p.batch, func(tx *gorm.DB, rows []OutboxEvent) error {
		// Group by store_id, computing max created_at per store.
		byStore := map[string]time.Time{}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: unparseable payload; dropping",
						"event_id", r.ID, "err", err)
				}
				continue
			}
			sid, _ := payload["store_id"].(string)
			if sid == "" {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: payload missing store_id; dropping",
						"event_id", r.ID, "event_type", r.EventType)
				}
				continue
			}
			if prev, ok := byStore[sid]; !ok || r.CreatedAt.After(prev) {
				byStore[sid] = r.CreatedAt
			}
		}
		for sid, ts := range byStore {
			if err := tx.Exec(`
				INSERT INTO store_watermarks (store_id, products_updated_at)
				VALUES (?, ?)
				ON CONFLICT (store_id) DO UPDATE
					SET products_updated_at = GREATEST(
						store_watermarks.products_updated_at,
						EXCLUDED.products_updated_at)`, sid, ts).Error; err != nil {
				return err
			}
		}
		return p.repo.MarkPublishedInTx(tx, ids)
	})
}
