package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// The date the MERCHANT is shown must be the effective end, and days_remaining
// must be counted from it. A merchant granted an extension who still sees the
// old date has been told something false by their own dashboard.
func TestEnrichTrialBanner_UsesExtendedEnd(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	created := now.Add(-88 * 24 * time.Hour) // derived end: 2 days away
	extended := now.Add(20 * 24 * time.Hour) // real end: 20 days away

	sub := subscription.StoreSubscription{
		ID: uuid.New(), Status: subscription.StatusTrialing,
		CreatedAt: created, TrialEndsAt: &extended,
	}

	var resp SubscriptionResponse
	enrichTrialBanner(&resp, sub, now)

	require.NotNil(t, resp.TrialEndsAt)
	require.Equal(t, extended.UTC().Format("2006-01-02T15:04:05Z"), *resp.TrialEndsAt,
		"the merchant must be shown the extended end, not created_at + 90d")
	require.NotNil(t, resp.DaysRemainingInTrial)
	require.Equal(t, 20, *resp.DaysRemainingInTrial,
		"days_remaining must count to the effective end; the derived end would give 2")
}
