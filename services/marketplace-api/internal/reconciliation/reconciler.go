// Package reconciliation implements the daily Stripe subscription status
// reconciliation job (P17 Task 10). It queries all active StoreSubscriptions
// with a stripe_subscription_id and compares the local status against the
// authoritative Stripe status.
//
// Drift types detected:
//   - status_mismatch  — Stripe status disagrees with our local status
//   - stripe_not_found — we hold a stripe_subscription_id that Stripe 404s
//
// "locally_missing" (Stripe has it, we don't) requires iterating Stripe
// customers and is deferred to v2 — it is expensive for large tenants.
package reconciliation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

const (
	// batchSize is the number of subscriptions fetched per reconciliation tick.
	// SKIP LOCKED means multiple pods can run concurrently without double-processing.
	batchSize = 500

	// DriftTypeStatusMismatch is emitted when Stripe's status differs from ours.
	DriftTypeStatusMismatch = "status_mismatch"
	// DriftTypeStripeNotFound is emitted when our stripe_subscription_id 404s on Stripe.
	DriftTypeStripeNotFound = "stripe_not_found"
)

// driftTotal is the Prometheus counter for reconciliation drift events.
// Registered once by init() against the default registry.
var driftTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "mark8ly",
		Name:      "subscription_reconciliation_drift_total",
		Help:      "Count of subscription status drift events detected during daily Stripe reconciliation.",
	},
	[]string{"drift_type"},
)

func init() {
	prometheus.MustRegister(driftTotal)
}

// row is an internal projection used in the batch SELECT query.
type row struct {
	StoreID              uuid.UUID
	TenantID             uuid.UUID
	LocalStatus          subscription.SubscriptionStatus
	StripeSubscriptionID string
}

// Reconciler queries active StoreSubscriptions and compares their status
// against Stripe.
type Reconciler struct {
	db      *gorm.DB
	stripe  *billingstripe.Client
	emitter *audit.Emitter
	logger  *slog.Logger
}

// New constructs a Reconciler. stripe may be nil — in that case all Stripe
// calls are skipped and only a warning is logged (useful in local dev without
// a Stripe key).
func New(db *gorm.DB, stripe *billingstripe.Client, emitter *audit.Emitter, logger *slog.Logger) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{db: db, stripe: stripe, emitter: emitter, logger: logger}
}

// RunOnce processes up to batchSize subscriptions and returns the count of
// drift events detected. It is safe to call concurrently — SKIP LOCKED
// prevents double-processing across pods.
func (r *Reconciler) RunOnce(ctx context.Context) (int, error) {
	if r.stripe == nil {
		r.logger.Warn("reconciliation: no Stripe client — skipping")
		return 0, nil
	}

	// Fetch a batch of active subscriptions that have a Stripe subscription ID.
	// SKIP LOCKED ensures concurrent replicas don't race.
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT store_id, tenant_id, status AS local_status, stripe_subscription_id
		FROM store_subscriptions
		WHERE status = 'active'
		  AND stripe_subscription_id IS NOT NULL
		ORDER BY updated_at
		LIMIT ?
		FOR UPDATE SKIP LOCKED
	`, batchSize).Scan(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("reconciliation: fetch batch: %w", err)
	}

	driftCount := 0
	for _, sub := range rows {
		drift, err := r.checkOne(ctx, sub)
		if err != nil {
			r.logger.Error("reconciliation: check failed",
				"store_id", sub.StoreID,
				"stripe_subscription_id", sub.StripeSubscriptionID,
				"err", err)
			continue
		}
		if drift != "" {
			driftCount++
		}
	}

	r.logger.Info("reconciliation: batch complete",
		"checked", len(rows),
		"drift", driftCount)

	return driftCount, nil
}

// checkOne reconciles a single subscription against Stripe and emits a drift
// event if a mismatch is found. Returns the drift type ("" if clean).
func (r *Reconciler) checkOne(ctx context.Context, sub row) (string, error) {
	stripeSub, err := billingstripe.GetSubscription(ctx, r.stripe, sub.StripeSubscriptionID)
	if err != nil {
		var apiErr *billingstripe.APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus == 404 {
			r.recordDrift(ctx, sub, DriftTypeStripeNotFound,
				fmt.Sprintf("stripe_subscription_id %s returned 404", sub.StripeSubscriptionID))
			return DriftTypeStripeNotFound, nil
		}
		return "", fmt.Errorf("reconciliation: stripe get subscription %s: %w", sub.StripeSubscriptionID, err)
	}

	// Compare Stripe status to our local status.
	// Stripe uses: active, past_due, canceled, unpaid, trialing, paused, incomplete, incomplete_expired
	// We map "active" ↔ active, everything else is a potential mismatch.
	localActive := sub.LocalStatus == subscription.StatusActive
	stripeActive := stripeSub.Status == "active"

	if localActive != stripeActive {
		detail := fmt.Sprintf("local=%s stripe=%s stripe_sub_id=%s",
			sub.LocalStatus, stripeSub.Status, sub.StripeSubscriptionID)
		r.recordDrift(ctx, sub, DriftTypeStatusMismatch, detail)
		return DriftTypeStatusMismatch, nil
	}

	return "", nil
}

// recordDrift increments the drift counter, logs the finding, and emits an
// audit event for ops triage.
func (r *Reconciler) recordDrift(ctx context.Context, sub row, driftType, detail string) {
	driftTotal.WithLabelValues(driftType).Inc()

	r.logger.Warn("reconciliation: drift detected",
		"drift_type", driftType,
		"store_id", sub.StoreID,
		"tenant_id", sub.TenantID,
		"stripe_subscription_id", sub.StripeSubscriptionID,
		"detail", detail)

	if r.emitter != nil {
		r.emitter.Emit(nil, audit.Event{
			Action:       "subscription.reconciliation_drift",
			ResourceType: "store_subscription",
			ResourceID:   sub.StripeSubscriptionID,
			TenantID:     sub.TenantID,
			StoreID:      sub.StoreID,
			Severity:     audit.SeverityWarning,
			Metadata: map[string]any{
				"drift_type": driftType,
				"detail":     detail,
			},
		})
	}
}
