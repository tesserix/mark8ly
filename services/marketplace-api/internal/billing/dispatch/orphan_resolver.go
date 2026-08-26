package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// OrphanConfig holds the dependencies required by OrphanResolver.
type OrphanConfig struct {
	DB         *gorm.DB
	Repo       webhookevents.Repository
	Dispatcher *Dispatcher
	MaxRetries int
	BatchSize  int
}

// OrphanResolver fetches webhook events with no store_id (orphans) and
// attempts to resolve them by looking up the Stripe customer ID against
// store_subscriptions. Resolved events are dispatched normally.
type OrphanResolver struct {
	cfg OrphanConfig
}

// NewOrphanResolver constructs an OrphanResolver. MaxRetries defaults to 6
// and BatchSize defaults to 50 when zero-valued.
func NewOrphanResolver(cfg OrphanConfig) *OrphanResolver {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 6
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 50
	}
	return &OrphanResolver{cfg: cfg}
}

// RunOnce fetches up to BatchSize orphan events and attempts to resolve each.
// Failed resolutions bump retry_count; reaching MaxRetries flips manual_review_required.
func (r *OrphanResolver) RunOnce(ctx context.Context) error {
	orphans, err := r.cfg.Repo.GetUnprocessedOrphans(ctx, r.cfg.DB, r.cfg.BatchSize)
	if err != nil {
		return err
	}
	for _, e := range orphans {
		if err := r.resolveOne(ctx, e); err != nil {
			newCount, rerr := r.cfg.Repo.IncrementRetry(ctx, r.cfg.DB, e.EventID, err.Error())
			if rerr != nil {
				return rerr
			}
			if newCount >= r.cfg.MaxRetries {
				_ = r.cfg.Repo.FlagManualReview(ctx, r.cfg.DB, e.EventID, "retry cap exceeded")
			}
		}
	}
	return nil
}

func (r *OrphanResolver) resolveOne(ctx context.Context, e webhookevents.StripeWebhookEvent) error {
	storeID, tenantID, ok := lookupStoreByStripeCustomerPayload(ctx, r.cfg.DB, []byte(e.Payload))
	if !ok {
		return fmt.Errorf("orphan: no subscription for event_id=%s", e.EventID)
	}
	if err := r.cfg.Repo.SetStoreID(ctx, r.cfg.DB, e.EventID, storeID, tenantID); err != nil {
		return err
	}
	// Collector installed before the lock, drained after it commits — the
	// dispatcher registers provider HTTP calls (e.g. the trial-billed
	// confirmation) here rather than making them under the advisory lock.
	ctx, deferred := WithDeferredSends(ctx)
	err := subscription.WithAdvisoryLock(ctx, r.cfg.DB, storeID, func(tx *gorm.DB) error {
		return r.cfg.Dispatcher.Dispatch(ctx, tx, e)
	})
	if err != nil {
		// Rolled back: the pending sends describe side effects that never
		// happened, so drop them rather than draining.
		return err
	}
	// Non-fatal by contract: a failed email must not un-process the event.
	for _, sendErr := range deferred.Run(ctx) {
		slog.Default().Warn("orphan: deferred billing email failed",
			"event_id", e.EventID, "err", sendErr.Error())
	}
	return r.cfg.Repo.MarkProcessed(ctx, r.cfg.DB, e.EventID)
}

// lookupStoreByStripeCustomerPayload parses customer ID from raw event JSON and
// resolves (store_id, tenant_id) via store_subscriptions.stripe_customer_id.
// Duplicates the logic from handlers/webhooks/stripe.go rather than importing
// it to avoid a dispatcher <- handlers import cycle.
func lookupStoreByStripeCustomerPayload(ctx context.Context, db *gorm.DB, payload []byte) (uuid.UUID, uuid.UUID, bool) {
	var e struct {
		Data struct {
			Object struct {
				Customer string `json:"customer"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &e); err != nil || e.Data.Object.Customer == "" {
		return uuid.Nil, uuid.Nil, false
	}
	var row struct {
		StoreID  string `gorm:"column:store_id"`
		TenantID string `gorm:"column:tenant_id"`
	}
	err := db.WithContext(ctx).Raw(
		`SELECT store_id::text AS store_id, tenant_id::text AS tenant_id
         FROM store_subscriptions
         WHERE stripe_customer_id = ?
         LIMIT 1`,
		e.Data.Object.Customer,
	).Scan(&row).Error
	if err != nil || row.StoreID == "" {
		return uuid.Nil, uuid.Nil, false
	}
	sid, err := uuid.Parse(row.StoreID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	tid, err := uuid.Parse(row.TenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return sid, tid, true
}

// Sentinel for documentation — callers check nil-ness of returned uuid.
var _ = errors.New
