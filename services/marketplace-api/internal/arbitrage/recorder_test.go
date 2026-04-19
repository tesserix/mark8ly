//go:build integration

package arbitrage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// spyCounter satisfies arbitrage.Counter and records calls.
type spyCounter struct {
	flaggedCalls int
	clearedCalls int
}

func (s *spyCounter) IncArbitrageFlagged()              { s.flaggedCalls++ }
func (s *spyCounter) IncArbitrageFalsePositiveCleared() { s.clearedCalls++ }

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
