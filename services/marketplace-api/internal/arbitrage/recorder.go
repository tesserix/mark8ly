package arbitrage

import (
	"context"
	"errors"
	"fmt"

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
//  2. Inserts a subscription_arbitrage_audit row.
//  3. Updates store_subscriptions.arbitrage_flag = true.
//  4. Increments the Prometheus counter.
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

	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("insert arbitrage audit: %w", err)
		}

		res := tx.Model(&subscription.StoreSubscription{}).
			Where("id = ? AND tenant_id = ?", in.SubscriptionID, in.TenantID).
			Update("arbitrage_flag", true)
		if res.Error != nil {
			return fmt.Errorf("toggle arbitrage_flag: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return errors.New("arbitrage: subscription not found or tenant mismatch")
		}
		return nil
	}); err != nil {
		return err
	}

	// Increment counter only after a successful commit.
	r.count.IncArbitrageFlagged()
	return nil
}
