//go:build integration

package arbitrage_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionArbitrageAudit_NoRawIPColumn(t *testing.T) {
	db := testdb.NewDB(t, "subscription_arbitrage_audit")

	var cnt int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
                               WHERE table_name='subscription_arbitrage_audit'
                                 AND column_name IN ('raw_ip','ip','client_ip')`).Scan(&cnt).Error)
	require.EqualValues(t, 0, cnt, "raw IP columns must not exist")
}

func TestSubscriptionArbitrageAudit_RoundTrip(t *testing.T) {
	db := testdb.NewDB(t, "subscription_arbitrage_audit", "store_subscriptions")

	sub := subscription.StoreSubscription{
		TenantID: uuid.New(), StoreID: uuid.New(),
		StripeCustomerID: "cus_x", Plan: subscription.PlanStudio, Status: subscription.StatusActive,
	}
	require.NoError(t, db.Create(&sub).Error)

	ipCountry, ipHash := "IN", "abc123"
	row := arbitrage.SubscriptionArbitrageAudit{
		SubscriptionID: sub.ID, TenantID: sub.TenantID, StoreID: sub.StoreID,
		IPCountry: &ipCountry, IPHash: &ipHash,
		ResolvedPriceTier: "ppp",
	}
	require.NoError(t, db.Create(&row).Error)

	var got arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.First(&got, "id=?", row.ID).Error)
	require.Equal(t, arbitrage.ResolutionOngoing, got.Resolution)
}
