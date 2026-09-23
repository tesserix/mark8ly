// Package webhooks hosts Gin handlers for external-system callbacks.
// stripe.go verifies Stripe webhook signatures on the raw body, stores the
// event idempotently, and dispatches inside an advisory-lock transaction.
package webhooks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/postcommit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// DispatchFunc is supplied by main.go — routes an event to the per-type
// handler inside the advisory-locked transaction. In P2 this calls into
// internal/billing/dispatch.
type DispatchFunc func(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error

// StripeHandlerConfig holds all injected dependencies for StripeHandler.
// DB, Secret, Repo, Dispatch, and AllowedTypes are required.
type StripeHandlerConfig struct {
	DB           *gorm.DB
	Secret       string
	Repo         webhookevents.Repository
	Dispatch     DispatchFunc
	AllowedTypes map[string]bool
	MaxBodyBytes int64
	// MaxRetries bounds how many failed dispatches are tolerated before the
	// event is flagged for manual review instead of asking Stripe to
	// redeliver. Defaults to defaultMaxRetries; set it from the same config
	// value as the orphan cron (OrphanRetryMaxCount) so the two agree.
	MaxRetries int
	Now        func() time.Time
	Logger     *slog.Logger
}

// defaultMaxRetries mirrors dispatch.NewOrphanResolver's own default.
const defaultMaxRetries = 6

// StripeHandler is the Gin handler for POST /webhooks/stripe-billing.
type StripeHandler struct {
	cfg StripeHandlerConfig
}

// NewStripeHandler fills defaults and returns a handler. DB, Secret, Repo,
// Dispatch, and AllowedTypes are required.
func NewStripeHandler(cfg StripeHandlerConfig) *StripeHandler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = 512 << 10 // 512 KB
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &StripeHandler{cfg: cfg}
}

// Handle processes an inbound Stripe webhook POST. It:
//  1. Caps request body at MaxBodyBytes to prevent OOM attacks.
//  2. Verifies the Stripe-Signature header on the raw bytes.
//  3. Inserts the event idempotently. A redelivery of an event that already
//     processed returns 200 "duplicate"; a redelivery of one that never
//     processed is dispatched again.
//  4. Skips dispatch for event types not in AllowedTypes — stamps them
//     processed and returns 200 "persisted".
//  5. Resolves stripe_customer_id → store_id, acquires advisory lock,
//     dispatches, and stamps processed_at in the same transaction.
//  6. On dispatch error bumps retry_count and returns 503 so Stripe
//     redelivers; past MaxRetries it flags manual review and returns 200 so
//     Stripe stops.
//
// All log calls use sanitized fields only — the raw body is never logged.
func (h *StripeHandler) Handle(c *gin.Context) {
	// 1. Cap body size.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.cfg.MaxBodyBytes)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "body_too_large"})
		return
	}

	// 2. Verify signature on raw bytes BEFORE any parsing.
	sig := c.GetHeader("Stripe-Signature")
	eventID, eventType, err := billingstripe.VerifySignature(raw, sig, h.cfg.Secret, h.cfg.Now())
	if err != nil {
		h.cfg.Logger.Warn("stripe: signature verification failed", "err", err.Error())
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
		return
	}

	allowed := h.cfg.AllowedTypes[eventType]

	// 3. Idempotent insert — event_id is the natural idempotency key.
	evt := webhookevents.StripeWebhookEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   datatypes.JSON(raw),
	}
	inserted, err := h.cfg.Repo.InsertIfNew(c.Request.Context(), h.cfg.DB, evt)
	if err != nil {
		h.cfg.Logger.Error("stripe: InsertIfNew failed",
			"event_id", eventID, "event_type", eventType, "err", err.Error())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "persist_failed"})
		return
	}
	if !inserted {
		// A redelivery is not automatically a duplicate of WORK done. Stripe
		// resends for up to three days, and answering every resend "duplicate"
		// without looking meant a row whose first dispatch failed could never
		// be retried by Stripe — the one retry mechanism that costs us
		// nothing to run. Only an event that actually processed is finished.
		existing, getErr := h.cfg.Repo.Get(c.Request.Context(), h.cfg.DB, eventID)
		switch {
		case getErr != nil:
			h.cfg.Logger.Error("stripe: could not load the existing event",
				"event_id", eventID, "event_type", eventType, "err", getErr.Error())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
			return
		case existing.ProcessedAt != nil:
			h.cfg.Logger.Info("stripe: duplicate of an already-processed event ignored",
				"event_id", eventID, "event_type", eventType)
			c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
			return
		case existing.ManualReviewRequired, existing.RetryCount >= h.maxRetries():
			// Out of automatic road. Answering 200 stops Stripe from
			// resending something only a human can now move, and the row
			// stays visible to an operator via cmd/webhook-replay.
			h.cfg.Logger.Warn("stripe: redelivery of an event awaiting manual review",
				"event_id", eventID, "event_type", eventType,
				"retry_count", existing.RetryCount)
			c.JSON(http.StatusOK, gin.H{"status": "manual_review_required"})
			return
		}
		h.cfg.Logger.Info("stripe: redelivery of an unprocessed event — retrying dispatch",
			"event_id", eventID, "event_type", eventType, "retry_count", existing.RetryCount)
		evt = *existing
	}

	// 4. Check event-type allowlist. Persisted for audit, never dispatched —
	// and stamped processed, because it IS finished: there is no handler and
	// no later attempt that could change that.
	//
	// It used to be left unprocessed, which quietly enrolled every ignored
	// event in the recovery loop: attributed, dispatched, "no handler for
	// <type>", retried to the cap, flagged. Harmless-looking noise that
	// consumed the same retry budget and alert channel as a real failure —
	// and with recovery no longer blind to attributed events, it would now
	// page as well.
	if !allowed {
		if err := h.cfg.Repo.MarkProcessed(c.Request.Context(), h.cfg.DB, eventID); err != nil {
			h.cfg.Logger.Warn("stripe: could not stamp an ignored event as processed",
				"event_id", eventID, "event_type", eventType, "err", err.Error())
		}
		h.cfg.Logger.Info("stripe: event_type not in allowlist",
			"event_id", eventID, "event_type", eventType)
		c.JSON(http.StatusOK, gin.H{"status": "persisted"})
		return
	}

	// 5. Resolve customer → store_id, acquire advisory lock, dispatch.
	// Orphan path: if the stripe_customer_id has no matching store_subscriptions row yet
	// (e.g. checkout.session.completed arrives before the DB row is fully committed),
	// dispatchLocked returns an error. We route through IncrementRetry so the event
	// stays with processed_at = NULL, store_id NULL, and processing_error carrying the
	// orphan reason — visible to the resolver via GetUnprocessed and to an operator.
	// MarkProcessed is NOT called, satisfying the SLA.
	if err := h.dispatchLocked(c.Request.Context(), evt); err != nil {
		count, _ := h.cfg.Repo.IncrementRetry(
			c.Request.Context(), h.cfg.DB, eventID, billingstripe.SanitizeForLog(err))

		// Ask Stripe to send it again. This used to answer 200, which threw
		// away three days of free exponential retries and left the in-process
		// cron as the only recovery — a cron that could not see this event at
		// all once store_id was set. Two retry mechanisms were nominally in
		// place and neither ran.
		if count < h.maxRetries() {
			h.cfg.Logger.Error("stripe: dispatch failed — asking Stripe to redeliver",
				"event_id", eventID, "event_type", eventType, "retry_count", count,
				"err", billingstripe.SanitizeForLog(err))
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "dispatch_failed"})
			return
		}

		// Past the cap, stop the churn: flag it, take the 200, and let an
		// operator pick it up. Stripe would otherwise keep resending an
		// event that has failed the same way every time.
		_ = h.cfg.Repo.FlagManualReview(
			c.Request.Context(), h.cfg.DB, eventID, "retry cap exceeded at the webhook endpoint")
		h.cfg.Logger.Error("stripe: dispatch failed past the retry cap — flagged for manual review",
			"event_id", eventID, "event_type", eventType, "retry_count", count,
			"err", billingstripe.SanitizeForLog(err))
		c.JSON(http.StatusOK, gin.H{"status": "manual_review_required"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// maxRetries is the number of failed attempts tolerated before an event is
// handed to a human. It matches the orphan cron's cap so an event cannot be
// flagged by one path while the other still considers it live.
func (h *StripeHandler) maxRetries() int {
	if h.cfg.MaxRetries > 0 {
		return h.cfg.MaxRetries
	}
	return defaultMaxRetries
}

// dispatchLocked resolves the customer → store_id mapping and, if found,
// runs the dispatcher inside an advisory-lock transaction. Orphan events
// (no matching store_subscription yet) return an error so the handler routes
// through IncrementRetry — the event stays with processed_at = NULL and
// store_id = NULL, making it visible to the cron retrier via GetUnprocessedOrphans.
func (h *StripeHandler) dispatchLocked(ctx context.Context, evt webhookevents.StripeWebhookEvent) error {
	storeID, tenantID, ok := lookupStoreByStripeCustomer(ctx, h.cfg.DB, evt.EventType, []byte(evt.Payload))
	if !ok {
		// Orphan: no store_subscriptions row for this stripe_customer_id yet.
		// Return an error rather than nil so that IncrementRetry is called in Handle,
		// leaving store_id NULL for the cron to resolve and providing a processing_error
		// field that operators can inspect. MarkProcessed is NOT called.
		return errors.New("dispatch: no store_subscription for stripe customer (orphan)")
	}
	if err := h.cfg.Repo.SetStoreID(ctx, h.cfg.DB, evt.EventID, storeID, tenantID); err != nil {
		return err
	}
	// Collector installed before the lock, drained after it commits. The
	// dispatcher registers provider HTTP calls (the trial-billed
	// confirmation) on it instead of making them inline: a SendGrid call
	// (15s, plus a possible Resend fallback) held the per-store advisory
	// lock and one connection of a small pool against Stripe's 30s webhook
	// budget.
	ctx, deferred := postcommit.WithDeferredSends(ctx)
	if err := subscription.WithAdvisoryLock(ctx, h.cfg.DB, storeID, func(tx *gorm.DB) error {
		if derr := h.cfg.Dispatch(ctx, tx, evt); derr != nil {
			return derr
		}
		// Stamped inside the dispatch transaction. Stamping it afterwards
		// left a window where the effects were committed and the row still
		// read unprocessed — harmless while nothing ever retried such a row,
		// and not harmless now that Stripe redelivery and the recovery loop
		// both re-attempt one.
		return h.cfg.Repo.MarkProcessed(ctx, tx, evt.EventID)
	}); err != nil {
		// Rolled back: the pending sends describe side effects that never
		// happened, so drop them rather than draining.
		return err
	}

	// Non-fatal by contract: returning a send failure here would make Stripe
	// retry the event and re-fire every other side effect.
	for _, sendErr := range deferred.Run(ctx) {
		h.cfg.Logger.Warn("stripe: deferred billing email failed",
			"event_id", evt.EventID, "event_type", evt.EventType,
			"err", billingstripe.SanitizeForLog(sendErr))
	}
	return nil
}

// lookupStoreByStripeCustomer resolves the event's customer to a store.
// Returns (Nil, Nil, false) when the payload carries no customer id or no
// subscription matches it.
//
// The extraction lives in webhookevents.CustomerIDFromPayload, shared with
// the orphan resolver. The two used to keep separate copies of this parsing —
// and both copies read only data.object.customer, so neither could route a
// customer.updated event, whose object IS the customer.
func lookupStoreByStripeCustomer(ctx context.Context, db *gorm.DB, eventType string, payload []byte) (uuid.UUID, uuid.UUID, bool) {
	customerID := webhookevents.CustomerIDFromPayload(eventType, payload)
	if customerID == "" {
		return uuid.Nil, uuid.Nil, false
	}
	return webhookevents.LookupStoreByCustomer(ctx, db, customerID)
}
