package arbitrage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// AllowlistFunc is the billing-ops-approved predicate for clearing a flag.
// Production is backed by a Secret Manager-managed list; tests inject a
// predicate directly. Returning true means "clear this flag".
type AllowlistFunc func(subscriptionID uuid.UUID) bool

// AllowAll is a convenience AllowlistFunc that approves every subscription.
// Use in dev/test environments only.
func AllowAll(_ uuid.UUID) bool { return true }

// QuarterlyAudit clears stale `ongoing` flags under per-store advisory locks
// so merchant-initiated appeals cannot race with the batch clear (§18.8
// concurrency rule).
type QuarterlyAudit struct {
	db         *gorm.DB
	allowlist  AllowlistFunc
	counter    Counter
	piiLogger  PIILogger
	maxAgeDays int
}

// NewQuarterlyAudit constructs a QuarterlyAudit with a 90-day default cutoff.
func NewQuarterlyAudit(db *gorm.DB, allowlist AllowlistFunc, counter Counter, pii PIILogger) *QuarterlyAudit {
	return &QuarterlyAudit{
		db:         db,
		allowlist:  allowlist,
		counter:    counter,
		piiLogger:  pii,
		maxAgeDays: 90,
	}
}

// WithMaxAgeDays overrides the default 90-day cutoff. Useful for tests.
func (a *QuarterlyAudit) WithMaxAgeDays(days int) *QuarterlyAudit {
	a.maxAgeDays = days
	return a
}

// Run walks every `ongoing` audit row older than maxAgeDays. For each:
//  1. Checks the allowlist; if not approved, skips.
//  2. Acquires the per-store advisory lock (same helper as subscription mutations).
//  3. Sets resolution = false_positive_cleared + arbitrage_flag = false.
//  4. Increments the false-positive-cleared counter.
//
// Per-row errors are logged and skipped — one bad row must not block the batch.
func (a *QuarterlyAudit) Run(ctx context.Context) error {
	cutoff := time.Now().Add(-time.Duration(a.maxAgeDays) * 24 * time.Hour)

	var rows []SubscriptionArbitrageAudit
	if err := a.db.WithContext(ctx).
		Where("resolution = ? AND flagged_at < ?", string(ResolutionOngoing), cutoff).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("quarterly audit: load ongoing rows: %w", err)
	}

	for _, row := range rows {
		if !a.allowlist(row.SubscriptionID) {
			continue
		}

		// Log PII access before processing each row.
		a.piiLogger.LogPIIAccess(ctx, PIIAccessEvent{
			Actor:     uuid.Nil, // system job
			StoreID:   row.StoreID,
			TenantID:  row.TenantID,
			Operation: "arbitrage_quarterly_clear",
		})

		row := row // capture loop variable for closure
		err := subscription.WithAdvisoryLock(ctx, a.db, row.StoreID, func(tx *gorm.DB) error {
			now := time.Now().UTC()
			if err := tx.Model(&SubscriptionArbitrageAudit{}).
				Where("id = ?", row.ID).
				Updates(map[string]any{
					"resolution":  string(ResolutionFalsePositiveCleared),
					"reviewed_at": now,
				}).Error; err != nil {
				return fmt.Errorf("update audit row %s: %w", row.ID, err)
			}
			return tx.Model(&subscription.StoreSubscription{}).
				Where("id = ? AND tenant_id = ?", row.SubscriptionID, row.TenantID).
				Update("arbitrage_flag", false).Error
		})
		if err != nil {
			a.piiLogger.LogPIIAccess(ctx, PIIAccessEvent{
				Actor:     uuid.Nil,
				StoreID:   row.StoreID,
				TenantID:  row.TenantID,
				Operation: "arbitrage_quarterly_clear_failed",
				Note:      err.Error(),
			})
			continue
		}
		a.counter.IncArbitrageFalsePositiveCleared()
	}
	return nil
}
