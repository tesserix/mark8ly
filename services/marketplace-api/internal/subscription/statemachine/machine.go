package statemachine

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

var (
	// ErrInvalidTransition is returned when the requested (From → To) pair is
	// not in the transition table defined by §17.2.
	ErrInvalidTransition = errors.New("statemachine: invalid transition")

	// ErrCASConflict is returned when the CAS UPDATE matches zero rows —
	// meaning another writer already moved the subscription out of From.
	// Callers that replay idempotent webhooks should treat this as a silent
	// no-op.
	ErrCASConflict = errors.New("statemachine: CAS conflict (status changed concurrently)")
)

// TransitionInput describes a desired state change. DB and the four identity
// + direction fields are required. GinCtx and StripeEventID are optional.
type TransitionInput struct {
	DB      *gorm.DB
	Emitter *audit.Emitter

	// GinCtx is optional — used by EmitStateTransition to extract IP, UA,
	// and user identity for the audit row. Pass nil for background callers
	// (cron, webhooks) — the event will be tagged as ActorSystem.
	GinCtx *gin.Context

	TenantID uuid.UUID
	StoreID  uuid.UUID

	// From is the expected current status and serves as the CAS anchor.
	From subscription.SubscriptionStatus
	To   subscription.SubscriptionStatus

	// Actor identifies who initiated the transition. Conventions:
	//   "user:<uuid>"                  — merchant action
	//   "system:webhook:stripe"        — Stripe webhook dispatcher
	//   "system:cron:trial_expiry"     — scheduled job
	Actor string

	// Reason is a free-text label that normally mirrors the webhook event
	// name or cron job name. Stored in the audit metadata for traceability.
	Reason string

	// StripeEventID is optional. When present it is included in the audit
	// metadata so each transition can be traced back to the originating
	// Stripe event.
	StripeEventID string
}

// Transition is the only sanctioned entry point for changing
// store_subscriptions.status outside raw migrations. It:
//
//  1. Validates (From → To) against the §17.2 transition table.
//  2. Acquires a pg_advisory_xact_lock on the store to prevent concurrent
//     writers from racing on the same subscription row.
//  3. Executes a CAS UPDATE (WHERE status = From) and returns ErrCASConflict
//     when zero rows are affected — indicating a concurrent writer already
//     moved the status.
//  4. Emits an audit event via Emitter on success. Audit failure is
//     non-fatal: it is logged and swallowed so a transient audit outage
//     never blocks a legitimate state transition.
func Transition(ctx context.Context, in TransitionInput) error {
	if !IsValidTransition(in.From, in.To) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, in.From, in.To)
	}

	return subscription.WithAdvisoryLock(ctx, in.DB, in.StoreID, func(tx *gorm.DB) error {
		res := tx.Exec(`
			UPDATE store_subscriptions
			SET status = ?, updated_at = now()
			WHERE tenant_id = ?
			  AND store_id  = ?
			  AND status    = ?`,
			in.To, in.TenantID, in.StoreID, in.From,
		)
		if res.Error != nil {
			return fmt.Errorf("statemachine: CAS update: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrCASConflict
		}

		// Audit emit is fire-and-forget: a nil emitter (e.g. in unit tests
		// that opt out) is safe to call — EmitStateTransition is a no-op
		// when the receiver is nil.
		if in.Emitter != nil {
			in.Emitter.EmitStateTransition(in.GinCtx, audit.StateTransition{
				StoreID:       in.StoreID,
				TenantID:      in.TenantID,
				From:          string(in.From),
				To:            string(in.To),
				Actor:         in.Actor,
				Reason:        in.Reason,
				StripeEventID: in.StripeEventID,
			})
		}

		// Metrics: emit only when the package-level collector is initialised.
		// Unit tests that construct machine.go without the metrics package
		// (e.g. integration tests with isolated DBs) are safely skipped.
		if metrics.Subscription != nil {
			metrics.Subscription.SubscriptionStateTransitionedTotal.
				WithLabelValues(string(in.From), string(in.To), in.Reason).Inc()
		}

		return nil
	})
}
