package arbitrage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Counter is the observability interface for arbitrage metrics.
// Production wires to metrics.Subscription; tests inject a spy.
// See P17 for alert rules over these counters.
type Counter interface {
	IncArbitrageFlagged()
	IncArbitrageFalsePositiveCleared()
	// IncArbitrageTenantMismatch counts refused audit writes where the
	// caller's tenant does not own the subscription. This is never routine —
	// it is a caller bug or a cross-tenant probe — so it is counted
	// separately from a successful flag and alerted on (#423).
	IncArbitrageTenantMismatch()
}

// RecordInput carries every field needed for a triangulation decision and
// subsequent audit-row write.
type RecordInput struct {
	SubscriptionID uuid.UUID
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	PriceTier      subscription.PriceTier
	CardCountry    string
	BillingCountry string
	IPCountry      string
	RawIP          string // hashed then dropped; NEVER persisted or logged directly
}

// ErrTenantMismatch is returned when the tenant that the caller supplied does
// not own the subscription being flagged. No audit row is written on this path
// — see RecordIfFlagged for why persisting it would be worse than losing it.
var ErrTenantMismatch = errors.New("arbitrage: subscription not found or tenant mismatch")

// Recorder persists arbitrage audit rows and toggles the arbitrage_flag on
// the subscription. It is the only component that writes to
// subscription_arbitrage_audit — all other packages query it read-only.
type Recorder struct {
	db     *gorm.DB
	hasher *Hasher
	count  Counter
}

// NewRecorder constructs a Recorder. count may not be nil.
func NewRecorder(db *gorm.DB, hasher *Hasher, count Counter) *Recorder {
	return &Recorder{db: db, hasher: hasher, count: count}
}

// RecordIfFlagged evaluates the three country signals against the price tier.
// When the decision is a flag it:
//  1. Hashes rawIP with HMAC-SHA256 (never persisting the plaintext).
//  2. Verifies the caller's tenant owns the subscription.
//  3. Inserts a subscription_arbitrage_audit row on its own statement.
//  4. Updates store_subscriptions.arbitrage_flag = true on a separate one.
//  5. Increments the Prometheus counter.
//
// Steps 3 and 4 are deliberately NOT one transaction. See the inline comments:
// the audit row must outlive a failed flag toggle, and must never be written
// when step 2 says the tenant does not own the subscription (#423).
//
// On a clean decision it is a no-op (no DB writes, no counter increment).
//
// Stripe redelivers on non-nil returns so this method is safe to call twice —
// duplicate audit rows are permitted and grouped by subscription_id in billing-ops.
func (r *Recorder) RecordIfFlagged(ctx context.Context, in RecordInput) error {
	dec := Evaluate(Input{
		PriceTier:      in.PriceTier,
		CardCountry:    in.CardCountry,
		BillingCountry: in.BillingCountry,
		IPCountry:      in.IPCountry,
	})
	if !dec.Flagged {
		return nil
	}

	hash, err := r.hasher.Hash(ctx, in.RawIP)
	if err != nil {
		// Hashing failed (Secret Manager unavailable). Prefer to persist the
		// flag with an empty ip_hash rather than silently drop the signal —
		// billing-ops still has card/ip/billing country to act on.
		hash = HashResult{}
	}

	cardNorm := NormalizeCountry(in.CardCountry)
	billingNorm := NormalizeCountry(in.BillingCountry)
	ipNorm := NormalizeCountry(in.IPCountry)

	db := r.db.WithContext(ctx)

	// Step 1 — ownership check. The audit table carries tenant_id and store_id
	// with no foreign key, so nothing in the schema stops a fraud row being
	// written under a tenant that does not own the subscription. billing-ops
	// filters the inbox on tenant_id alone (internal/inbox/arbitrage.go), so
	// such a row would surface the case in the WRONG tenant's queue. That is
	// strictly worse than losing the row: it leaks one merchant's fraud signal
	// to another. So a mismatch refuses the write outright — but loudly, since
	// it is a caller bug or a cross-tenant probe, never routine (#423).
	var owned int64
	if err := db.Model(&subscription.StoreSubscription{}).
		Where("id = ? AND tenant_id = ?", in.SubscriptionID, in.TenantID).
		Count(&owned).Error; err != nil {
		return fmt.Errorf("arbitrage: verify subscription ownership: %w", err)
	}
	if owned == 0 {
		slog.Default().Error("arbitrage: refusing audit row — subscription not found or tenant mismatch",
			"subscription_id", in.SubscriptionID.String(),
			"tenant_id", in.TenantID.String(),
			"store_id", in.StoreID.String(),
			"price_tier", string(in.PriceTier))
		r.count.IncArbitrageTenantMismatch()
		return ErrTenantMismatch
	}

	row := SubscriptionArbitrageAudit{
		SubscriptionID:    in.SubscriptionID,
		TenantID:          in.TenantID,
		StoreID:           in.StoreID,
		ResolvedPriceTier: string(in.PriceTier),
		Resolution:        ResolutionOngoing,
	}
	if cardNorm != "??" {
		row.CardCountry = &cardNorm
	}
	if billingNorm != "??" {
		row.BillingCountry = &billingNorm
	}
	if ipNorm != "??" {
		row.IPCountry = &ipNorm
	}
	if hash.Hex != "" {
		row.IPHash = &hash.Hex
	}
	if dec.MismatchReason != "" {
		row.MismatchReason = &dec.MismatchReason
	}

	// Step 2 — the audit row is committed on its OWN statement, deliberately
	// NOT sharing a transaction with the flag toggle below. Previously both ran
	// inside one transaction, so any error from the toggle rolled the audit row
	// back and the fraud case was lost with no log, no metric and no row (#423).
	// The FK to store_subscriptions is satisfied because step 1 just proved the
	// parent exists.
	//
	// Duplicate rows from a Stripe redelivery are expected and harmless — see
	// the method doc: billing-ops groups by subscription_id.
	if err := db.Create(&row).Error; err != nil {
		return fmt.Errorf("insert arbitrage audit: %w", err)
	}

	// Step 3 — toggle the denormalised convenience flag. If this fails the
	// audit row above is already durable, which is the whole point: losing the
	// flag degrades one admin screen (internal/handlers/admin/subscription.go),
	// losing the row loses the fraud case. The error is still returned so
	// Stripe redelivers and the toggle is retried.
	res := db.Model(&subscription.StoreSubscription{}).
		Where("id = ? AND tenant_id = ?", in.SubscriptionID, in.TenantID).
		Update("arbitrage_flag", true)
	if res.Error != nil {
		return fmt.Errorf("toggle arbitrage_flag: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// The subscription vanished between step 1 and here (a concurrent
		// hard delete). The audit row is already committed and that is the
		// outcome we want; the ON DELETE CASCADE will reap it if the parent
		// really is gone. Report so the caller logs and Stripe redelivers.
		slog.Default().Error("arbitrage: flag toggle matched no subscription after ownership check",
			"subscription_id", in.SubscriptionID.String(),
			"tenant_id", in.TenantID.String())
		r.count.IncArbitrageTenantMismatch()
		return ErrTenantMismatch
	}

	// Increment counter only after the audit row is durable.
	r.count.IncArbitrageFlagged()
	return nil
}
