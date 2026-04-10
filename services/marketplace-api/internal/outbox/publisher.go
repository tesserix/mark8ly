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
//
// Watermark routing: rows whose aggregate is one of {order, return,
// abandoned_cart} bump store_watermarks.orders_updated_at; everything else
// (product, category, media) bumps products_updated_at. This lets storefront
// clients poll the orders signal independently of product edits. See spec
// §14.1 and Orders M2 plan (Option A).
func (p *Publisher) Tick(ctx context.Context) (int, error) {
	return p.repo.ProcessBatch(ctx, p.batch, func(tx *gorm.DB, rows []OutboxEvent) error {
		// Group by (store_id, axis) where axis ∈ {"products","orders"}.
		type key struct {
			storeID string
			axis    string
		}
		byBucket := map[key]time.Time{}
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
			axis := "products"
			if IsOrderAggregate(r.Aggregate) {
				axis = "orders"
			}
			k := key{storeID: sid, axis: axis}
			if prev, ok := byBucket[k]; !ok || r.CreatedAt.After(prev) {
				byBucket[k] = r.CreatedAt
			}
		}
		for k, ts := range byBucket {
			var stmt string
			switch k.axis {
			case "orders":
				stmt = `
					INSERT INTO store_watermarks (store_id, orders_updated_at)
					VALUES (?, ?)
					ON CONFLICT (store_id) DO UPDATE
						SET orders_updated_at = GREATEST(
							store_watermarks.orders_updated_at,
							EXCLUDED.orders_updated_at)`
			default:
				stmt = `
					INSERT INTO store_watermarks (store_id, products_updated_at)
					VALUES (?, ?)
					ON CONFLICT (store_id) DO UPDATE
						SET products_updated_at = GREATEST(
							store_watermarks.products_updated_at,
							EXCLUDED.products_updated_at)`
			}
			if err := tx.Exec(stmt, k.storeID, ts).Error; err != nil {
				return err
			}
		}
		return p.repo.MarkPublishedInTx(tx, ids)
	})
}
