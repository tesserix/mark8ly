//go:build integration

package arbitrage_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// spyCounter satisfies arbitrage.Counter and records calls.
type spyCounter struct {
	flaggedCalls  int
	clearedCalls  int
	mismatchCalls int
}

func (s *spyCounter) IncArbitrageFlagged()              { s.flaggedCalls++ }
func (s *spyCounter) IncArbitrageFalsePositiveCleared() { s.clearedCalls++ }
func (s *spyCounter) IncArbitrageTenantMismatch()       { s.mismatchCalls++ }

// stubVersionsSource implements arbitrage.VersionsSource for test use.
type stubVersionsSource struct {
	versions []arbitrage.KeyVersion
}

func (s *stubVersionsSource) ListEnabled(_ context.Context) ([]arbitrage.KeyVersion, error) {
	return s.versions, nil
}

func makeTestHasher(t *testing.T) *arbitrage.Hasher {
	t.Helper()
	src := &stubVersionsSource{
		versions: []arbitrage.KeyVersion{
			{Name: "v1", Payload: []byte("test-key-32-bytes-padded--------"), CreatedAt: time.Now()},
		},
	}
	loader := arbitrage.NewKeyLoader(src, 5*time.Minute)
	return arbitrage.NewHasher(loader)
}

func TestRecorder_FlagWritesAuditRowAndTogglesFlag(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, makeTestHasher(t), spy)

	err := rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "203.0.113.9",
	})
	require.NoError(t, err)

	var row arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("subscription_id = ?", subID).First(&row).Error)
	require.NotNil(t, row.IPHash)
	require.Len(t, *row.IPHash, 64)
	require.Equal(t, arbitrage.ResolutionOngoing, row.Resolution)
	require.NotNil(t, row.MismatchReason)
	require.Contains(t, *row.MismatchReason, "card_country=US")

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", subID).First(&sub).Error)
	require.True(t, sub.ArbitrageFlag)
	require.Equal(t, 1, spy.flaggedCalls)
}

func TestRecorder_NoFlagIsNoOp(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedDevelopedSubscription(t, db)

	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, makeTestHasher(t), spy)

	err := rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierDeveloped,
		CardCountry:    "US",
		BillingCountry: "US",
		IPCountry:      "US",
		RawIP:          "1.2.3.4",
	})
	require.NoError(t, err)

	var count int64
	db.Model(&arbitrage.SubscriptionArbitrageAudit{}).Where("subscription_id = ?", subID).Count(&count)
	require.Zero(t, count)
	require.Zero(t, spy.flaggedCalls)
}

func TestRecorder_IncrementsCounter(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, makeTestHasher(t), spy)

	_ = rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "GB",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "2.2.2.2",
	})
	require.Equal(t, 1, spy.flaggedCalls)
}

// --- #423 regression tests -------------------------------------------------
//
// Both failure paths of RecordIfFlagged used to share one transaction with the
// audit-row insert, so any failure discarded the fraud row with no log, no
// metric and no trace. They now get different answers, and each is pinned here.

// failStoreSubscriptionUpdate injects a DB error on UPDATEs of
// store_subscriptions only, leaving the audit-row INSERT working. That is
// exactly the shape of the old recorder.go:115 path.
func failStoreSubscriptionUpdate(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Callback().Update().Before("gorm:update").
		Register("test:fail_store_subscription_update", func(tx *gorm.DB) {
			if tx.Statement != nil && tx.Statement.Table == "store_subscriptions" {
				tx.AddError(errors.New("injected: toggle failed"))
			}
		})
	require.NoError(t, err)
}

// TestRecorder_AuditRowSurvivesFailedFlagToggle pins the #423 fix for the
// toggle-error path: the flag is lost, the fraud row is NOT.
func TestRecorder_AuditRowSurvivesFailedFlagToggle(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)
	failStoreSubscriptionUpdate(t, db)

	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, makeTestHasher(t), spy)

	err := rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "203.0.113.9",
	})
	require.Error(t, err, "a failed flag toggle must still be reported to the caller")
	require.Contains(t, err.Error(), "toggle arbitrage_flag")

	// The whole point of #423: the audit row outlives the failed toggle.
	var count int64
	require.NoError(t, db.Model(&arbitrage.SubscriptionArbitrageAudit{}).
		Where("subscription_id = ?", subID).Count(&count).Error)
	require.Equal(t, int64(1), count, "audit row must survive a failed flag toggle")

	var row arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("subscription_id = ?", subID).First(&row).Error)
	require.Equal(t, tenantID, row.TenantID)
	require.NotNil(t, row.MismatchReason)

	// The flag is the acceptable casualty — it degrades one admin screen.
	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", subID).First(&sub).Error)
	require.False(t, sub.ArbitrageFlag)

	// Not a successful flag, so the success counter must not move.
	require.Zero(t, spy.flaggedCalls)
}

// TestRecorder_TenantMismatchWritesNothingAndIsLoud pins the #423 fix for the
// mismatch path: the rollback is kept (a row under the wrong tenant would leak
// into that tenant's billing-ops inbox) but it is logged and counted.
func TestRecorder_TenantMismatchWritesNothingAndIsLoud(t *testing.T) {
	db := openIntegrationDB(t)
	_, storeID, subID := seedPPPSubscription(t, db)
	wrongTenant := uuid.New()

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, makeTestHasher(t), spy)

	err := rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       wrongTenant,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "203.0.113.9",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, arbitrage.ErrTenantMismatch)

	// No row at all — not under the wrong tenant, not under the right one.
	var count int64
	require.NoError(t, db.Model(&arbitrage.SubscriptionArbitrageAudit{}).
		Where("subscription_id = ?", subID).Count(&count).Error)
	require.Zero(t, count, "a tenant mismatch must not persist an audit row")

	var mismatchRows int64
	require.NoError(t, db.Model(&arbitrage.SubscriptionArbitrageAudit{}).
		Where("tenant_id = ?", wrongTenant).Count(&mismatchRows).Error)
	require.Zero(t, mismatchRows, "no row may be written under the non-owning tenant")

	// Loud: counted...
	require.Equal(t, 1, spy.mismatchCalls, "tenant mismatch must increment its own counter")
	require.Zero(t, spy.flaggedCalls)

	// ...and logged, with enough identifiers to chase the caller down.
	logged := logBuf.String()
	require.Contains(t, logged, "tenant mismatch")
	require.Contains(t, logged, subID.String())
	require.Contains(t, logged, wrongTenant.String())
	// The raw IP must never reach the log.
	require.NotContains(t, logged, "203.0.113.9")
}
