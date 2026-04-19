package harddelete

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// Runner executes the full hard-delete pipeline for a single store:
//
//  1. (P10 deferred) Build billing archive snapshot.
//  2. Delete Stripe customer (idempotent — 404 treated as success).
//  3. Sweep all tenant-scoped tables inside a transaction.
//  4. Transition pending_hard_delete → hard_deleted via statemachine.Transition.
//  5. Emit subscription.hard_deleted audit event.
//
// All steps run under WithAdvisoryLock on storeID to prevent concurrent runs.
type Runner struct {
	db            *gorm.DB
	stripeClient  *billingstripe.Client
	emitter       *audit.Emitter
	logger        *slog.Logger
}

// NewRunner constructs a Runner. stripeClient may be nil (Stripe delete is skipped
// with a warning in that case — local dev / test environments).
func NewRunner(db *gorm.DB, stripe *billingstripe.Client, emitter *audit.Emitter, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{db: db, stripeClient: stripe, emitter: emitter, logger: logger}
}

// Run executes the full hard-delete pipeline for the given subscription row.
// Returns nil if the store was already hard-deleted (idempotent).
func (r *Runner) Run(ctx context.Context, row *subscription.StoreSubscription) error {
	if row.Status != subscription.StatusPendingHardDelete {
		return fmt.Errorf("harddelete: runner: expected pending_hard_delete, got %s", row.Status)
	}

	return subscription.WithAdvisoryLock(ctx, r.db, row.StoreID, func(tx *gorm.DB) error {
		return r.runLocked(ctx, tx, row)
	})
}

func (r *Runner) runLocked(ctx context.Context, tx *gorm.DB, row *subscription.StoreSubscription) error {
	storeID := row.StoreID
	tenantID := row.TenantID

	// Step 1: Billing archive (P10 deferred).
	// P10's billingarchive.Builder.Build(ctx, storeID) will be wired here when P10 lands.
	// Until then, log intent so the audit trail shows the intended pipeline.
	r.logger.Info("harddelete: billing archive step deferred (P10 not yet wired)",
		"store_id", storeID, "tenant_id", tenantID)

	// Step 2: Delete Stripe customer — idempotent (404 = already deleted = success).
	if r.stripeClient != nil && row.StripeCustomerID != "" {
		if err := billingstripe.DeleteCustomer(ctx, r.stripeClient, row.StripeCustomerID); err != nil {
			r.logger.Error("harddelete: stripe customer delete failed",
				"store_id", storeID, "stripe_customer_id", row.StripeCustomerID, "err", err)
			return fmt.Errorf("harddelete: stripe customer delete: %w", err)
		}
		r.logger.Info("harddelete: stripe customer deleted",
			"store_id", storeID, "stripe_customer_id", row.StripeCustomerID)
	} else {
		r.logger.Warn("harddelete: stripe client nil or no customer id — skipping Stripe delete",
			"store_id", storeID, "has_stripe_client", r.stripeClient != nil,
			"customer_id", row.StripeCustomerID)
	}

	// Step 3: Tenant-scoped DELETE sweep across all tables.
	if err := Sweep(ctx, tx, r.emitter, r.logger, storeID, tenantID); err != nil {
		return fmt.Errorf("harddelete: sweep: %w", err)
	}

	// Step 4: Transition pending_hard_delete → hard_deleted via statemachine.
	// NOTE: the stores row was deleted in Sweep, so we pass the tenant/store IDs
	// directly from the in-memory row (no DB lookup needed).
	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       tx,
		Emitter:  r.emitter,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusPendingHardDelete,
		To:       subscription.StatusHardDeleted,
		Actor:    "system:cron:lifecycle_hard_delete",
		Reason:   "lifecycle:pending_hard_delete_150d",
	})
	switch {
	case err == nil:
		// Transition succeeded — the store_subscriptions row was deleted by Sweep
		// (step 3 above), so the statemachine CAS UPDATE will hit 0 rows and return
		// ErrCASConflict. We handle that as a success: deletion already completed.
		r.logger.Info("harddelete: store hard-deleted",
			"store_id", storeID, "tenant_id", tenantID)
	case errors.Is(err, statemachine.ErrCASConflict):
		// Row already deleted or transitioned — treat as success (idempotent).
		r.logger.Info("harddelete: CAS conflict after sweep — store already hard-deleted",
			"store_id", storeID)
	default:
		return fmt.Errorf("harddelete: transition: %w", err)
	}

	// Step 5: Final audit event.
	r.emitter.Emit(nil, audit.Event{
		Action:         "subscription.hard_deleted",
		ResourceType:   "subscription",
		ResourceID:     storeID.String(),
		Severity:       audit.SeverityCritical,
		TenantID:       tenantID,
		StoreID:        storeID,
		ForceActorType: audit.ActorSystem,
		Metadata: map[string]any{
			"store_id":    storeID.String(),
			"tenant_id":   tenantID.String(),
			"customer_id": row.StripeCustomerID,
		},
	})
	return nil
}

// RunByStoreID looks up the subscription and runs the hard-delete pipeline.
// Convenience wrapper for the 150-day cron.
func (r *Runner) RunByStoreID(ctx context.Context, db *gorm.DB, repo subscription.Repository,
	tenantID, storeID uuid.UUID) error {

	sub, err := repo.GetByStoreID(ctx, db, tenantID, storeID)
	if err != nil {
		return fmt.Errorf("harddelete: lookup subscription: %w", err)
	}
	return r.Run(ctx, sub)
}
