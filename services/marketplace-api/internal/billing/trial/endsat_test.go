package trial_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// An unextended trial ends created_at + TrialDays. The fixture uses a
// created_at that is NOT a round number of days from any boundary, so an
// implementation that truncated to a day would give a different answer.
func TestEndsAt_UnextendedDerivesFromCreatedAt(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created}

	got := trial.EndsAt(sub)

	require.Equal(t, created.Add(trial.TrialDays*24*time.Hour).UTC(), got)
}

// An extended trial ends at the stored value. The stored value is
// deliberately NOT created_at + 90d — an implementation that ignored the
// column would return the derived date, and this asserts it does not.
func TestEndsAt_ExtendedUsesStoredValue(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	extended := time.Date(2026, 9, 30, 8, 15, 0, 0, time.UTC)
	require.NotEqual(t, created.Add(trial.TrialDays*24*time.Hour).UTC(), extended.UTC(),
		"fixture must distinguish the stored value from the derived one")

	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created, TrialEndsAt: &extended}

	require.Equal(t, extended.UTC(), trial.EndsAt(sub))
}

// A trial can be extended BACKWARDS (shortened). Nothing in the accessor
// should assume the stored value is later than the derived one.
func TestEndsAt_StoredValueEarlierThanDerivedIsHonoured(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	earlier := created.Add(10 * 24 * time.Hour)
	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created, TrialEndsAt: &earlier}

	require.Equal(t, earlier.UTC(), trial.EndsAt(sub))
}

// The return is always UTC, whatever the driver hands back.
func TestEndsAt_AlwaysUTC(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+1800)
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, loc)
	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created}

	require.Equal(t, time.UTC, trial.EndsAt(sub).Location())

	stored := time.Date(2026, 9, 30, 8, 15, 0, 0, loc)
	sub.TrialEndsAt = &stored
	require.Equal(t, time.UTC, trial.EndsAt(sub).Location())
}
