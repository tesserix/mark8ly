package dispatch

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/postcommit"
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

// RunOnce fetches up to BatchSize unprocessed events and attempts each one.
// Failed attempts bump retry_count; reaching MaxRetries flips manual_review_required.
//
// "Unprocessed" now means exactly that, rather than "unprocessed AND still
// an orphan". The handler sets store_id before it dispatches, so an event
// whose handler failed came back with store_id populated and fell outside
// the old query — and Stripe had been answered 200, so no other retry
// existed. Those events were stranded silently and permanently.
func (r *OrphanResolver) RunOnce(ctx context.Context) error {
	pending, err := r.cfg.Repo.GetUnprocessed(ctx, r.cfg.DB, r.cfg.BatchSize)
	if err != nil {
		return err
	}
	for _, e := range pending {
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
	// An event that already knows its store is not an orphan — it is a
	// previous dispatch failure — so it needs no lookup, only another
	// attempt. Re-resolving would be wrong as well as wasteful: the store
	// on the row is what the first attempt acted under.
	storeID, ok := e.ResolvedStoreID()
	if !ok {
		var err error
		if storeID, err = r.resolveStore(ctx, e); err != nil {
			return err
		}
	}

	// Collector installed before the lock, drained after it commits — the
	// dispatcher registers provider HTTP calls (e.g. the trial-billed
	// confirmation) here rather than making them under the advisory lock.
	ctx, deferred := postcommit.WithDeferredSends(ctx)
	err := subscription.WithAdvisoryLock(ctx, r.cfg.DB, storeID, func(tx *gorm.DB) error {
		if derr := r.cfg.Dispatcher.Dispatch(ctx, tx, e); derr != nil {
			return derr
		}
		// Marked inside the dispatch transaction, not after it. Stamping it
		// afterwards left a window where the effects were committed and the
		// row still said unprocessed, so the next run would apply them
		// again — a window this change would otherwise have widened, since
		// re-attempting a dispatched event is now something that happens.
		return r.cfg.Repo.MarkProcessed(ctx, tx, e.EventID)
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
	return nil
}

// resolveStore looks the event's Stripe customer up against
// store_subscriptions and records the result on the row.
func (r *OrphanResolver) resolveStore(ctx context.Context, e webhookevents.StripeWebhookEvent) (uuid.UUID, error) {
	storeID, tenantID, ok := lookupStoreByStripeCustomerPayload(ctx, r.cfg.DB, e.EventType, []byte(e.Payload))
	if !ok {
		return uuid.Nil, fmt.Errorf("orphan: no subscription for event_id=%s", e.EventID)
	}
	if err := r.cfg.Repo.SetStoreID(ctx, r.cfg.DB, e.EventID, storeID, tenantID); err != nil {
		return uuid.Nil, err
	}
	return storeID, nil
}

// lookupStoreByStripeCustomerPayload resolves an event's customer to a store.
//
// This used to hold its own copy of the payload parsing, duplicated from
// handlers/webhooks/stripe.go to dodge an import cycle — and both copies read
// only data.object.customer, so neither could resolve a customer.updated
// event and both retried it to the manual-review cap. The extraction now
// lives once, in webhookevents, which both packages already import.
func lookupStoreByStripeCustomerPayload(ctx context.Context, db *gorm.DB, eventType string, payload []byte) (uuid.UUID, uuid.UUID, bool) {
	customerID := webhookevents.CustomerIDFromPayload(eventType, payload)
	if customerID == "" {
		return uuid.Nil, uuid.Nil, false
	}
	return webhookevents.LookupStoreByCustomer(ctx, db, customerID)
}
