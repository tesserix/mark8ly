//go:build integration

package webhook_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newSub(t *testing.T, repo *webhook.SubscriptionRepo, tenant uuid.UUID, events []string) *webhook.Subscription {
	t.Helper()
	s := &webhook.Subscription{
		TenantID:   tenant,
		StoreID:    uuid.New(),
		URL:        "https://hooks.example.com/x",
		EventTypes: events,
		Secret:     "s3cret-value-for-test",
		Enabled:    true,
	}
	require.NoError(t, repo.Create(context.Background(), s))
	return s
}

func TestMatchingEvent_ReturnsOnlyEnabledSubscriptionsWantingThatType(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	tenant := uuid.New()
	ctx := context.Background()

	want := newSub(t, repo, tenant, []string{"order.placed", "order.refunded"})
	newSub(t, repo, tenant, []string{"product.created"})  // wrong type
	newSub(t, repo, uuid.New(), []string{"order.placed"}) // wrong tenant
	disabled := newSub(t, repo, tenant, []string{"order.placed"})
	_, err := repo.RecordFailure(ctx, disabled.ID, 1) // threshold 1 → disabled
	require.NoError(t, err)

	got, err := repo.MatchingEvent(ctx, tenant, "order.placed")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, want.ID, got[0].ID)
}

func TestRecordFailure_DisablesAtThresholdAndRecordsWhy(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	ctx := context.Background()
	s := newSub(t, repo, uuid.New(), []string{"order.placed"})

	disabled, err := repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.False(t, disabled, "one failure must not disable a subscription")

	_, err = repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	disabled, err = repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.True(t, disabled, "third consecutive failure should disable")

	all, err := repo.ListForStore(ctx, s.TenantID, s.StoreID)
	require.NoError(t, err)
	require.False(t, all[0].Enabled)
	require.NotNil(t, all[0].DisabledReason, "merchant must be told why")
	require.NotNil(t, all[0].DisabledAt)
}

// A working delivery after a failure must clear the counter, or an endpoint
// that fails intermittently over weeks would eventually be disabled despite
// mostly working.
func TestRecordSuccess_ResetsTheFailureCounter(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	ctx := context.Background()
	s := newSub(t, repo, uuid.New(), []string{"order.placed"})

	_, err := repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.NoError(t, repo.RecordSuccess(ctx, s.ID))

	// Two more failures must not disable — the counter restarted.
	_, err = repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	disabled, err := repo.RecordFailure(ctx, s.ID, 3)
	require.NoError(t, err)
	require.False(t, disabled)
}
