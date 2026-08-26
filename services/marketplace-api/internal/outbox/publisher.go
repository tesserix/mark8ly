// Package outbox: publisher goroutine. See spec §14.1 and §14.6.
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"gorm.io/gorm"
)

// Publisher polls outbox_events and bumps store_watermarks asynchronously.
// See spec §14.1 (watermark separation) and §14.6 (publisher semantics).
//
// Payload invariant: every outbox row in slice 1 carries a "store_id" key
// at the top level of its JSON payload. A row without it, or with a payload
// that will not unmarshal, is logged and marked FAILED — error is set, and
// published_at is left NULL. It never blocks the publisher (the original
// reason for dropping it) and it is never retried (both causes are
// deterministic properties of the row), but it is no longer recorded as a
// successful publish. See #336; the failed state is served by #331.
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
//
// The returned int is the number of rows the poll SAW — not the number
// published. Since #336 a batch can be entirely failed, so a non-zero return
// says work was examined, not that anything was delivered. The
// outbox_events_published_total and outbox_events_failed_total counters are
// what separate those two outcomes.
func (p *Publisher) Tick(ctx context.Context) (int, error) {
	var published, failed int
	seen, err := p.repo.ProcessBatch(ctx, p.batch, func(tx *gorm.DB, rows []OutboxEvent) error {
		// Reset per attempt: ProcessBatch's callback can run again, and a
		// counter that survived a rolled-back attempt would over-count.
		published, failed = 0, 0
		// Group by (store_id, axis) where axis ∈ {"products","orders"}.
		type key struct {
			storeID string
			axis    string
		}
		byBucket := map[key]time.Time{}
		idsByStore := make(map[string][]string, len(rows))
		failures := make([]Failure, 0)
		for _, r := range rows {
			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: unparseable payload; failing",
						"event_id", r.ID, "err", err)
				}
				failures = append(failures, Failure{ID: r.ID, Reason: ReasonPayloadUnparseable})
				continue
			}
			sid, _ := payload["store_id"].(string)
			// A non-UUID value must be rejected HERE, not left to the store
			// pre-check below: stores.id is uuid, so passing "store-42" to that
			// SELECT raises `invalid input syntax for type uuid`, which ABORTS
			// the transaction and rolls back the whole batch — the very poison
			// pill this pre-check exists to remove.
			if _, err := uuid.Parse(sid); sid == "" || err != nil {
				if p.logger != nil {
					p.logger.Warn("outbox publisher: payload missing store_id; failing",
						"event_id", r.ID, "event_type", r.EventType)
				}
				failures = append(failures, Failure{ID: r.ID, Reason: ReasonPayloadMissingStoreID})
				continue
			}
			// Appended only now: a row reaches this line exactly when it is
			// going to contribute a watermark bump. Appending before the
			// checks above is what made a dropped event indistinguishable
			// from a published one (#336).
			idsByStore[sid] = append(idsByStore[sid], r.ID)
			axis := "products"
			if IsOrderAggregate(r.Aggregate) {
				axis = "orders"
			}
			k := key{storeID: sid, axis: axis}
			if prev, ok := byBucket[k]; !ok || r.CreatedAt.After(prev) {
				byBucket[k] = r.CreatedAt
			}
		}
		// Store-existence pre-check (#374). store_watermarks.store_id is
		// REFERENCES stores(id), so upserting a watermark for a store that
		// does not exist raises an FK violation — and an FK violation ABORTS
		// the Postgres transaction, so it does not fail one row, it takes the
		// whole batch: the good rows, the failure marks, everything. Those
		// rows then stay pending and are re-selected forever.
		//
		// Checking first turns that into a per-row terminal failure, the same
		// shape as the other two causes. One extra SELECT per tick, and only
		// when at least one row survived validation.
		if len(idsByStore) > 0 {
			storeIDs := make([]string, 0, len(idsByStore))
			for sid := range idsByStore {
				storeIDs = append(storeIDs, sid)
			}
			var found []struct{ ID string }
			if err := tx.Raw(`SELECT id FROM stores WHERE id IN ?`, storeIDs).
				Scan(&found).Error; err != nil {
				return err
			}
			present := make(map[string]struct{}, len(found))
			for _, f := range found {
				present[f.ID] = struct{}{}
			}
			for sid, rowIDs := range idsByStore {
				if _, ok := present[sid]; ok {
					continue
				}
				if p.logger != nil {
					p.logger.Warn("outbox publisher: store not found; failing",
						"store_id", sid, "events", len(rowIDs))
				}
				for _, id := range rowIDs {
					failures = append(failures, Failure{ID: id, Reason: ReasonStoreNotFound})
				}
				delete(idsByStore, sid)
			}
			// Drop every bucket whose store is gone, both axes.
			for k := range byBucket {
				if _, ok := present[k.storeID]; !ok {
					delete(byBucket, k)
				}
			}
		}

		ids := make([]string, 0, len(rows))
		for _, rowIDs := range idsByStore {
			ids = append(ids, rowIDs...)
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
		// Both marks run in the SAME transaction as the watermark bumps
		// above, so a batch's outcome commits whole or not at all.
		published, failed = len(ids), len(failures)
		if err := p.repo.MarkFailedInTx(tx, failures); err != nil {
			return err
		}
		return p.repo.MarkPublishedInTx(tx, ids)
	})
	if err != nil {
		return seen, err
	}
	// AFTER the commit, never inside the callback: the transaction can roll
	// back, and a Prometheus counter cannot be decremented, so an over-count
	// is permanent.
	metrics.OutboxEventsPublishedTotal.Add(float64(published))
	metrics.OutboxEventsFailedTotal.Add(float64(failed))
	return seen, nil
}
