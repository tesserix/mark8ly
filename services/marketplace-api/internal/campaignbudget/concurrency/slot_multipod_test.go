//go:build integration

package concurrency_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget/concurrency"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestAdvisoryLockSlot_LockIsVisibleAcrossConnectionPools falsifies the claim
// this implementation used to carry:
//
//	"Single-pod deployments only — each pod has its own Postgres session pool,
//	 so a lock held on pod A is not visible to pod B."
//
// That is not how advisory locks work. They are global to the database, not
// scoped to a connection or a pool: two sessions cannot hold the same key at
// once, whichever pod, process or pool they belong to. The old comment
// confused "each pod has its own connections" with "locks are per-pool", and
// on that mistaken basis warned against running more than one replica.
//
// Two independent *gorm.DB handles are two independent connection pools —
// the same separation two pods have. If the old claim were true, pool B would
// get its own three slots and this test would fail.
func TestAdvisoryLockSlot_LockIsVisibleAcrossConnectionPools(t *testing.T) {
	podA := testdb.NewDB(t) // stand-in for replica A
	podB := testdb.NewDB(t) // stand-in for replica B — separate pool
	require.NotSame(t, podA, podB, "the two pods must not share a pool")

	acqA := concurrency.NewAdvisoryLockAcquirer(podA)
	acqB := concurrency.NewAdvisoryLockAcquirer(podB)
	storeID := uuid.New()

	// Pod A takes every slot for this store.
	for i := 0; i < concurrency.MaxConcurrentSends; i++ {
		_, err := acqA.AcquireSlot(context.Background(), storeID)
		require.NoError(t, err, "pod A should win slot %d", i)
	}

	// Pod B must be refused — the cap is per store across the whole cluster,
	// not per replica.
	_, err := acqB.AcquireSlot(context.Background(), storeID)
	require.ErrorIs(t, err, concurrency.ErrTooManyConcurrentSends,
		"pod B must see pod A's locks; the cap is cluster-wide, not per-pod")

	// And a different store is still free from pod B, so the refusal above is
	// the cap doing its job rather than pod B being broken.
	_, err = acqB.AcquireSlot(context.Background(), uuid.New())
	require.NoError(t, err, "an unrelated store must still be acquirable from pod B")
}
