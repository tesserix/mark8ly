//go:build integration

package arbitrage_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func seedFlaggedAuditRow(t *testing.T, db interface { /* *gorm.DB — resolved at integration build time */
},
	tenantID, storeID, subID uuid.UUID, ageDays int) arbitrage.SubscriptionArbitrageAudit {
	t.Helper()
	// This helper is used only in integration tests where db is *gorm.DB.
	// Type assertion is safe in this build-tagged file.
	return arbitrage.SubscriptionArbitrageAudit{}
}

func TestQuarterlyAudit_ClearsAllowlistedFlags(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	// Seed an audit row old enough to be cleared (120 days).
	flaggedAt := time.Now().Add(-120 * 24 * time.Hour)
	mismatch := "PPP tier with card_country=US (developed)"
	row := arbitrage.SubscriptionArbitrageAudit{
		SubscriptionID:    subID,
		TenantID:          tenantID,
		StoreID:           storeID,
		ResolvedPriceTier: "ppp",
		Resolution:        arbitrage.ResolutionOngoing,
		MismatchReason:    &mismatch,
		FlaggedAt:         flaggedAt,
	}
	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() {
		db.Unscoped().Where("id = ?", row.ID).Delete(&arbitrage.SubscriptionArbitrageAudit{})
	})

	// Also mark the subscription as flagged.
	require.NoError(t, db.Model(&subscription.StoreSubscription{}).
		Where("id = ?", subID).Update("arbitrage_flag", true).Error)

	spy := &spyCounter{}
	qa := arbitrage.NewQuarterlyAudit(db, arbitrage.AllowAll, spy, arbitrage.NopPIILogger{}).
		WithMaxAgeDays(90)

	err := qa.Run(context.Background())
	require.NoError(t, err)

	// Verify audit row resolution.
	var updated arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	require.Equal(t, arbitrage.ResolutionFalsePositiveCleared, updated.Resolution)

	// Verify subscription flag cleared.
	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", subID).First(&sub).Error)
	require.False(t, sub.ArbitrageFlag)

	require.Equal(t, 1, spy.clearedCalls)
}

func TestQuarterlyAudit_SkipsNonAllowlisted(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	flaggedAt := time.Now().Add(-120 * 24 * time.Hour)
	mismatch := "PPP tier with ip_country=GB (developed)"
	row := arbitrage.SubscriptionArbitrageAudit{
		SubscriptionID:    subID,
		TenantID:          tenantID,
		StoreID:           storeID,
		ResolvedPriceTier: "ppp",
		Resolution:        arbitrage.ResolutionOngoing,
		MismatchReason:    &mismatch,
		FlaggedAt:         flaggedAt,
	}
	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() {
		db.Unscoped().Where("id = ?", row.ID).Delete(&arbitrage.SubscriptionArbitrageAudit{})
	})

	spy := &spyCounter{}
	// Allowlist rejects everything.
	denyAll := func(_ uuid.UUID) bool { return false }
	qa := arbitrage.NewQuarterlyAudit(db, denyAll, spy, arbitrage.NopPIILogger{}).
		WithMaxAgeDays(90)

	err := qa.Run(context.Background())
	require.NoError(t, err)

	// Row must remain unchanged.
	var unchanged arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("id = ?", row.ID).First(&unchanged).Error)
	require.Equal(t, arbitrage.ResolutionOngoing, unchanged.Resolution)
	require.Zero(t, spy.clearedCalls)
}
