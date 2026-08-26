// Package webhooks hosts Gin handlers for external-system callbacks.
// stripe.go verifies Stripe webhook signatures on the raw body, stores the
// event idempotently, and dispatches inside an advisory-lock transaction.
package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
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
	Now          func() time.Time
	Logger       *slog.Logger
}

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
//  3. Inserts the event idempotently — duplicates return 200 "duplicate".
//  4. Skips dispatch for event types not in AllowedTypes — returns 200 "persisted".
//  5. Resolves stripe_customer_id → store_id, acquires advisory lock, dispatches.
//  6. On dispatch error bumps retry_count and returns 200 "retry_scheduled".
//  7. On success stamps processed_at and returns 200 "processed".
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
		h.cfg.Logger.Info("stripe: duplicate event ignored",
			"event_id", eventID, "event_type", eventType)
		c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
		return
	}

	// 4. Check event-type allowlist. Persisted but not dispatched.
	if !allowed {
		h.cfg.Logger.Info("stripe: event_type not in allowlist",
			"event_id", eventID, "event_type", eventType)
		c.JSON(http.StatusOK, gin.H{"status": "persisted"})
		return
	}

	// 5. Resolve customer → store_id, acquire advisory lock, dispatch.
	// Orphan path: if the stripe_customer_id has no matching store_subscriptions row yet
	// (e.g. checkout.session.completed arrives before the DB row is fully committed),
	// dispatchLocked returns an error. We route through IncrementRetry so the event
	// stays with processed_at = NULL and the cron can retry it once the row exists.
	// This is intentionally different from the "return nil" variant in the plan comment:
	// returning an error here means the cron will find it via GetUnprocessedOrphans
	// because store_id remains NULL and processing_error captures the orphan reason,
	// giving operators visibility. MarkProcessed is NOT called, satisfying the SLA.
	if err := h.dispatchLocked(c.Request.Context(), evt); err != nil {
		h.cfg.Logger.Error("stripe: dispatch failed (will retry via cron)",
			"event_id", eventID, "event_type", eventType,
			"err", billingstripe.SanitizeForLog(err))
		_, _ = h.cfg.Repo.IncrementRetry(
			c.Request.Context(), h.cfg.DB, eventID, billingstripe.SanitizeForLog(err))
		c.JSON(http.StatusOK, gin.H{"status": "retry_scheduled"})
		return
	}

	_ = h.cfg.Repo.MarkProcessed(c.Request.Context(), h.cfg.DB, eventID)
	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// dispatchLocked resolves the customer → store_id mapping and, if found,
// runs the dispatcher inside an advisory-lock transaction. Orphan events
// (no matching store_subscription yet) return an error so the handler routes
// through IncrementRetry — the event stays with processed_at = NULL and
// store_id = NULL, making it visible to the cron retrier via GetUnprocessedOrphans.
func (h *StripeHandler) dispatchLocked(ctx context.Context, evt webhookevents.StripeWebhookEvent) error {
	storeID, tenantID, ok := lookupStoreByStripeCustomer(ctx, h.cfg.DB, []byte(evt.Payload))
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
	ctx, deferred := dispatch.WithDeferredSends(ctx)
	if err := subscription.WithAdvisoryLock(ctx, h.cfg.DB, storeID, func(tx *gorm.DB) error {
		return h.cfg.Dispatch(ctx, tx, evt)
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

// lookupStoreByStripeCustomer parses the customer ID from the event payload
// and returns (store_id, tenant_id, true) if the store_subscriptions table
// has a matching row. Returns (Nil, Nil, false) on any parse or lookup failure.
func lookupStoreByStripeCustomer(ctx context.Context, db *gorm.DB, payload []byte) (uuid.UUID, uuid.UUID, bool) {
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
	storeUUID, err := uuid.Parse(row.StoreID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	tenantUUID, err := uuid.Parse(row.TenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return storeUUID, tenantUUID, true
}
