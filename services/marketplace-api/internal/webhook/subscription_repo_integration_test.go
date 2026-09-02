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
	return newSubForStore(t, repo, tenant, uuid.New(), events)
}

// newSubForStore is newSub with the store pinned. Fan-out matches on
// (tenant_id, store_id), so any test asserting who receives a delivery has
// to name the store rather than take a random one.
func newSubForStore(t *testing.T, repo *webhook.SubscriptionRepo, tenant, store uuid.UUID, events []string) *webhook.Subscription {
	t.Helper()
	s := &webhook.Subscription{
		TenantID:   tenant,
		StoreID:    store,
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

	store := uuid.New()
	want := newSubForStore(t, repo, tenant, store, []string{"order.placed", "order.refunded"})
	newSubForStore(t, repo, tenant, store, []string{"product.created"})   // wrong type
	newSubForStore(t, repo, uuid.New(), store, []string{"order.placed"})  // wrong tenant
	newSubForStore(t, repo, tenant, uuid.New(), []string{"order.placed"}) // wrong store
	disabled := newSubForStore(t, repo, tenant, store, []string{"order.placed"})
	_, err := repo.RecordFailure(ctx, disabled.ID, 1) // threshold 1 → disabled
	require.NoError(t, err)

	got, err := repo.MatchingEvent(ctx, tenant, store, "order.placed")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, want.ID, got[0].ID)
}

// The Critical review finding on the whole branch: webhook_subscriptions
// carries a NOT NULL store_id that scopes every merchant-facing read, but
// the fan-out match ignored it — so on a multi-store plan a webhook
// registered on Store A received Store B's events, with an
// identifier-only payload that gave the merchant no way to tell.
func TestMatchingEvent_DoesNotLeakAcrossStoresInOneTenant(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	tenant := uuid.New()
	storeA, storeB := uuid.New(), uuid.New()

	subA := newSubForStore(t, repo, tenant, storeA, []string{"order.placed"})
	newSubForStore(t, repo, tenant, storeB, []string{"order.placed"})

	got, err := repo.MatchingEvent(context.Background(), tenant, storeA, "order.placed")
	require.NoError(t, err)
	require.Len(t, got, 1, "a store B subscription must not match a store A event")
	require.Equal(t, subA.ID, got[0].ID)
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

// A further failure against an already-disabled subscription must not
// report a fresh disable, or Task 5's delivery worker would email the
// merchant "your webhook was disabled" on every subsequent failure against
// a dead endpoint instead of once.
func TestRecordFailure_AlreadyDisabledDoesNotReportAFreshDisable(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)
	ctx := context.Background()
	s := newSub(t, repo, uuid.New(), []string{"order.placed"})

	disabled, err := repo.RecordFailure(ctx, s.ID, 1)
	require.NoError(t, err)
	require.True(t, disabled, "first failure at threshold 1 disables and reports it")

	disabled, err = repo.RecordFailure(ctx, s.ID, 1)
	require.NoError(t, err)
	require.False(t, disabled, "already-disabled subscription must not report another disable")
}

func TestRecordFailure_UnknownIDDoesNotReportADisable(t *testing.T) {
	db := testdb.NewDB(t)
	repo := webhook.NewSubscriptionRepo(db)

	disabled, err := repo.RecordFailure(context.Background(), uuid.New(), 1)
	require.NoError(t, err)
	require.False(t, disabled, "an unknown id must not report a disable")
}
