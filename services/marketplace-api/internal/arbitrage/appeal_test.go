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

// spyPublisher records Publish calls.
type spyPublisher struct {
	calls   int
	lastMsg map[string]any
}

func (s *spyPublisher) Publish(_ context.Context, _ string, payload any) error {
	s.calls++
	if m, ok := payload.(map[string]any); ok {
		s.lastMsg = m
	}
	return nil
}

func TestAppealService_MarksAuditRowUnderReview(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	// First, record a flag.
	src := &stubVersionsSource{
		versions: []arbitrage.KeyVersion{
			{Name: "v1", Payload: []byte("key-32-bytes-exactly-padded-----"), CreatedAt: time.Now()},
		},
	}
	loader := arbitrage.NewKeyLoader(src, time.Minute)
	hasher := arbitrage.NewHasher(loader)
	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, hasher, spy)
	err := rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "1.2.3.4",
	})
	require.NoError(t, err)

	merchantUserID := uuid.New()
	pub := &spyPublisher{}
	svc := arbitrage.NewAppealService(db, pub, arbitrage.NopPIILogger{})

	err = svc.Submit(context.Background(), arbitrage.AppealInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		Jurisdiction:  "IN",
		Justification: "Our office is registered in India",
		DocumentURL:   "gs://mark8ly-docs/appeal-123.pdf",
		ActorUserID:   merchantUserID,
	})
	require.NoError(t, err)

	var row arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("subscription_id = ?", subID).First(&row).Error)
	require.Equal(t, &merchantUserID, row.ReviewedBy)
	require.NotNil(t, row.ReviewedAt)
	// Resolution stays ongoing — billing-ops closes it.
	require.Equal(t, arbitrage.ResolutionOngoing, row.Resolution)
	require.NotNil(t, row.MismatchReason)
	require.Contains(t, *row.MismatchReason, "MERCHANT_APPEAL")
	require.Contains(t, *row.MismatchReason, "IN")
}

func TestAppealService_RejectsNoOpenFlag(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID := uuid.New()
	storeID := uuid.New()

	svc := arbitrage.NewAppealService(db, arbitrage.NoOpPublisher{}, arbitrage.NopPIILogger{})
	err := svc.Submit(context.Background(), arbitrage.AppealInput{
		TenantID:     tenantID,
		StoreID:      storeID,
		Jurisdiction: "IN",
		ActorUserID:  uuid.New(),
	})
	require.ErrorIs(t, err, arbitrage.ErrNoOpenFlag)
}

func TestAppealService_PublishesToBillingOpsQueue(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	// Seed an audit row directly.
	mismatch := "PPP tier with card_country=US (developed)"
	row := arbitrage.SubscriptionArbitrageAudit{
		SubscriptionID:    subID,
		TenantID:          tenantID,
		StoreID:           storeID,
		ResolvedPriceTier: "ppp",
		Resolution:        arbitrage.ResolutionOngoing,
		MismatchReason:    &mismatch,
	}
	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() {
		db.Unscoped().Where("id = ?", row.ID).Delete(&arbitrage.SubscriptionArbitrageAudit{})
	})

	pub := &spyPublisher{}
	svc := arbitrage.NewAppealService(db, pub, arbitrage.NopPIILogger{})
	err := svc.Submit(context.Background(), arbitrage.AppealInput{
		TenantID:     tenantID,
		StoreID:      storeID,
		Jurisdiction: "IN",
		ActorUserID:  uuid.New(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, pub.calls, "expected one Publish call")
	require.Equal(t, subID, pub.lastMsg["subscription_id"])
	require.Equal(t, "IN", pub.lastMsg["jurisdiction"])
}
