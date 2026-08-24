package trial_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// The value sent to Stripe as trial_end must be the EFFECTIVE end. If this
// uses created_at + 90d, Stripe bills on a date the console does not show and
// the merchant is charged early.
//
// Asserting the VALUE, not that a call happened: a test that only checks a
// call was made passes against a fabricated zero.
func TestTrialEndValueSentToStripeUsesEffectiveEnd(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	extended := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	require.NotEqual(t, created.Add(trial.TrialDays*24*time.Hour).Unix(), extended.Unix(),
		"fixture must distinguish the extended value from the derived one")

	sub := subscription.StoreSubscription{
		ID: uuid.New(), CreatedAt: created, TrialEndsAt: &extended,
	}

	require.Equal(t, extended.UTC().Unix(), trial.EndsAt(sub).Unix(),
		"whatever computes the Stripe trial_end must go through EndsAt")
}
