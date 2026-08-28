//go:build integration

package arbitrage_test

import (
	"context"
	"strings"
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

// The worst case from #398: a DUAL-signal flag already carries a 68-char
// evaluator reason, so the 36-char appeal boilerplate alone exceeded
// varchar(100) before any merchant text. Plus a realistic long
// justification and doc URL, which the 1000-char service cap allows.
func TestAppealService_DualSignalFlag_LongAppeal_IsRecorded(t *testing.T) {
	db := openIntegrationDB(t)
	tenantID, storeID, subID := seedPPPSubscription(t, db)

	// Record a flag with BOTH signals: card_country and ip_country are each
	// a developed market, so Evaluate emits the 68-char dual-signal reason.
	src := &stubVersionsSource{
		versions: []arbitrage.KeyVersion{
			{Name: "v1", Payload: []byte("key-32-bytes-exactly-padded-----"), CreatedAt: time.Now()},
		},
	}
	loader := arbitrage.NewKeyLoader(src, time.Minute)
	hasher := arbitrage.NewHasher(loader)
	spy := &spyCounter{}
	rec := arbitrage.NewRecorder(db, hasher, spy)
	require.NoError(t, rec.RecordIfFlagged(context.Background(), arbitrage.RecordInput{
		SubscriptionID: subID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "GB",
		RawIP:          "1.2.3.4",
	}))

	var flagged arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("subscription_id = ?", subID).First(&flagged).Error)
	require.NotNil(t, flagged.MismatchReason)
	require.Contains(t, *flagged.MismatchReason, "card_country=US")
	require.Contains(t, *flagged.MismatchReason, "ip_country=GB")

	pub := &spyPublisher{}
	svc := arbitrage.NewAppealService(db, pub, arbitrage.NopPIILogger{})

	err := svc.Submit(context.Background(), arbitrage.AppealInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		ActorUserID:   uuid.New(),
		Jurisdiction:  "IN",
		Justification: strings.Repeat("Our registered office is in Bengaluru. ", 12),
		DocumentURL:   "gs://mark8ly-docs/appeals/2026/appeal-with-a-fairly-long-object-name-123456.pdf",
	})
	require.NoError(t, err, "a dual-signal flag with a realistic appeal must be recordable")

	var got arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("id = ?", flagged.ID).First(&got).Error)
	require.NotNil(t, got.MismatchReason)
	require.Contains(t, *got.MismatchReason, "MERCHANT_APPEAL")
	require.Contains(t, *got.MismatchReason, "jurisdiction=IN")
	require.Greater(t, len(*got.MismatchReason), 100,
		"this test is only meaningful if the value exceeds the old varchar(100) bound")
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
