// Package reconciliation implements the daily Stripe subscription status
// reconciliation job (P17 Task 10). It queries all active StoreSubscriptions
// with a stripe_subscription_id and compares the local status against the
// authoritative Stripe status.
//
// Drift types detected:
//   - status_mismatch  — Stripe status disagrees with our local status
//   - stripe_not_found — we hold a stripe_subscription_id that Stripe 404s
//   - locally_missing  — Stripe bills a subscription for one of our customers
//     that no local row records (#425: the plan-change transaction rolled
//     back after the Stripe call succeeded, orphaning the subscription)
//   - plan_mismatch    — the Stripe subscription item is on a different price
//     than the local (plan, period, currency, tier) resolves to (#425: the
//     Stripe price swap committed but the local plan write rolled back)
//
// The locally_missing scan is bounded, not a full customer crawl: it lists
// subscriptions only for customers we already know about, and only for rows
// whose stripe_subscription_id is NULL.
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
	// DriftTypeLocallyMissing is emitted when Stripe holds a subscription
	// tagged with one of our store ids that the local row does not record.
	DriftTypeLocallyMissing = "locally_missing"
	// DriftTypePlanMismatch is emitted when the Stripe subscription item's
	// price differs from the price our local plan/period/currency/tier
	// resolves to.
	DriftTypePlanMismatch = "plan_mismatch"

	// storeIDMetadataKey is the metadata key CreateSubscription stamps on
	// every subscription it creates (internal/billing/stripe/subscription_create.go).
	storeIDMetadataKey = "mark8ly_store_id"
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

// billablePlans are the plans that have a Stripe Price object in the catalog.
//
// This guard is load-bearing, not defensive tidiness: PriceIDFor reaches
// pricing.MustGetDescriptor, which PANICS for a (plan, period, tier) the
// catalog does not carry — and trial (the store_subscriptions default) and
// marketplace carry none. Broadening the batch to signup rows without this
// would crash the cron on the first trial row it met.
//
// For the three plans below the catalog defines both monthly and annual at
// the developed tier, so MustGetDescriptor is total over what can reach it;
// the PPP tier returns an error rather than panicking. A plan added here
// without catalog entries reintroduces the panic.
var billablePlans = map[subscription.SubscriptionPlan]bool{
	subscription.PlanStarter: true,
	subscription.PlanStudio:  true,
	subscription.PlanPro:     true,
}

// row is an internal projection used in the batch SELECT query.
//
// StripeSubscriptionID and BillingCurrency are nullable columns projected
// through COALESCE, so "" means NULL here.
type row struct {
	StoreID              uuid.UUID
	TenantID             uuid.UUID
	LocalStatus          subscription.SubscriptionStatus
	StripeCustomerID     string
	StripeSubscriptionID string
	Plan                 subscription.SubscriptionPlan
	SubscriptionPeriod   subscription.SubscriptionPeriod
	BillingCurrency      string
	PriceTier            subscription.PriceTier
}

// Reconciler queries active StoreSubscriptions and compares their status
// against Stripe.
type Reconciler struct {
	db      *gorm.DB
	stripe  *billingstripe.Client
	emitter *audit.Emitter
	logger  *slog.Logger

	// priceCache memoises PriceIDFor lookups for the life of the process.
	// Without it a 500-row batch issues 500 Stripe price lookups for what is
	// at most a handful of distinct (plan, period, currency, tier) tuples.
	// The reconciliation process is a short-lived CronJob, so a price
	// re-pointed mid-run is not a concern.
	priceCache map[string]string
}

// New constructs a Reconciler. stripe may be nil — in that case all Stripe
// calls are skipped and only a warning is logged (useful in local dev without
// a Stripe key).
func New(db *gorm.DB, stripe *billingstripe.Client, emitter *audit.Emitter, logger *slog.Logger) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{
		db:         db,
		stripe:     stripe,
		emitter:    emitter,
		logger:     logger,
		priceCache: make(map[string]string),
	}
}

// RunOnce processes up to batchSize subscriptions and returns the count of
// drift events detected. It is safe to call concurrently — SKIP LOCKED
// prevents double-processing across pods.
func (r *Reconciler) RunOnce(ctx context.Context) (int, error) {
	if r.stripe == nil {
		r.logger.Warn("reconciliation: no Stripe client — skipping")
		return 0, nil
	}

	// Fetch a batch of live subscriptions. signup and trialing are included
	// alongside active because the two drift types added for #425 are
	// invisible otherwise: an orphaned Stripe subscription belongs to a row
	// still sitting in signup, and a rolled-back upgrade can leave a
	// trialing row on the wrong price.
	//
	// Rows with a NULL stripe_subscription_id are included for the same
	// reason — that NULL is precisely the locally_missing signal — provided
	// we hold a customer id to ask Stripe about.
	//
	// SKIP LOCKED ensures concurrent replicas don't race.
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT store_id,
		       tenant_id,
		       status AS local_status,
		       stripe_customer_id,
		       COALESCE(stripe_subscription_id, '') AS stripe_subscription_id,
		       plan,
		       subscription_period,
		       COALESCE(billing_currency, '') AS billing_currency,
		       price_tier
		FROM store_subscriptions
		WHERE status IN ('active', 'signup', 'trialing')
		  AND (stripe_subscription_id IS NOT NULL OR stripe_customer_id <> '')
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
	// No local stripe_subscription_id: the only question worth asking is
	// whether Stripe is nonetheless billing one for this store.
	if sub.StripeSubscriptionID == "" {
		return r.checkLocallyMissing(ctx, sub)
	}

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

	// Status agrees — is the merchant on the price their local plan says?
	return r.checkPlanMismatch(ctx, sub, stripeSub), nil
}

// checkLocallyMissing asks Stripe whether it holds a live subscription for
// this store's customer that the local row does not record. A match is
// attributed by the mark8ly_store_id metadata CreateSubscription stamps, so a
// subscription created outside our flow is never mistaken for one of ours.
//
// Returns "" (and no error) when nothing can be concluded — an unknown
// customer, or a Stripe list call that failed — rather than failing the whole
// batch over one row.
func (r *Reconciler) checkLocallyMissing(ctx context.Context, sub row) (string, error) {
	if sub.StripeCustomerID == "" {
		return "", nil
	}

	stripeSubs, err := billingstripe.ListSubscriptionsByCustomer(ctx, r.stripe, sub.StripeCustomerID)
	if err != nil {
		return "", fmt.Errorf("reconciliation: stripe list subscriptions for customer %s: %w", sub.StripeCustomerID, err)
	}

	for _, candidate := range stripeSubs {
		if candidate == nil || candidate.Metadata[storeIDMetadataKey] != sub.StoreID.String() {
			continue
		}
		// Copy rather than mutate: recordDrift reports the Stripe id we
		// found, while the caller's row keeps its (empty) local value.
		found := sub
		found.StripeSubscriptionID = candidate.ID
		r.recordDrift(ctx, found, DriftTypeLocallyMissing,
			fmt.Sprintf("stripe has subscription %s (status=%s) for customer %s; local stripe_subscription_id is NULL",
				candidate.ID, candidate.Status, sub.StripeCustomerID))
		return DriftTypeLocallyMissing, nil
	}

	return "", nil
}

// checkPlanMismatch compares the price on the Stripe subscription item
// against the price the local (plan, period, currency, tier) resolves to.
//
// A price that cannot be resolved is not drift: the catalog legitimately has
// no entry for some combinations (a PPP currency without a PPP price, say),
// and reporting that as a plan change would bury the real signal. Same for a
// subscription Stripe returned with no items.
func (r *Reconciler) checkPlanMismatch(ctx context.Context, sub row, stripeSub *billingstripe.Subscription) string {
	if stripeSub == nil || len(stripeSub.Items.Data) == 0 {
		return ""
	}
	if !billablePlans[sub.Plan] {
		return ""
	}
	actualPriceID := stripeSub.Items.Data[0].Price.ID
	if actualPriceID == "" {
		return ""
	}

	expectedPriceID, err := r.expectedPriceID(ctx, sub)
	if err != nil || expectedPriceID == "" {
		r.logger.Debug("reconciliation: price lookup skipped",
			"store_id", sub.StoreID,
			"plan", sub.Plan,
			"period", sub.SubscriptionPeriod,
			"err", err)
		return ""
	}

	if actualPriceID == expectedPriceID {
		return ""
	}

	r.recordDrift(ctx, sub, DriftTypePlanMismatch,
		fmt.Sprintf("local plan=%s period=%s currency=%s tier=%s expects price %s; stripe subscription %s is on price %s",
			sub.Plan, sub.SubscriptionPeriod, sub.BillingCurrency, sub.PriceTier,
			expectedPriceID, sub.StripeSubscriptionID, actualPriceID))
	return DriftTypePlanMismatch
}

// expectedPriceID resolves the local plan to a Stripe price id, memoised per
// (plan, period, currency, tier).
func (r *Reconciler) expectedPriceID(ctx context.Context, sub row) (string, error) {
	key := fmt.Sprintf("%s|%s|%s|%s", sub.Plan, sub.SubscriptionPeriod, sub.BillingCurrency, sub.PriceTier)
	if cached, ok := r.priceCache[key]; ok {
		return cached, nil
	}

	priceID, err := billingstripe.PriceIDFor(ctx, r.stripe,
		sub.Plan, sub.SubscriptionPeriod, sub.BillingCurrency, sub.PriceTier)
	if err != nil {
		return "", err
	}
	if r.priceCache != nil {
		r.priceCache[key] = priceID
	}
	return priceID, nil
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
