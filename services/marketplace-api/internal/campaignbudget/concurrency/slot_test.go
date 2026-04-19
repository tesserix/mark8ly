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

// TestAdvisoryLockSlot_ThreeConcurrent verifies the advisory-lock acquirer
// enforces the 3-concurrent-send cap without Redis.
func TestAdvisoryLockSlot_ThreeConcurrent(t *testing.T) {
	db := testdb.NewDB(t)
	acq := concurrency.NewAdvisoryLockAcquirer(db)
	storeID := uuid.New()

	releases := make([]func(), 0, 3)
	for i := 0; i < 3; i++ {
		release, err := acq.AcquireSlot(context.Background(), storeID)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	_, err := acq.AcquireSlot(context.Background(), storeID)
	require.ErrorIs(t, err, concurrency.ErrTooManyConcurrentSends)

	// Release one and acquire again.
	releases[1]()
	_, err = acq.AcquireSlot(context.Background(), storeID)
	require.NoError(t, err)
}

// TestAdvisoryLockSlot_IsolatedByStore verifies that slots for different stores
// are independent — acquiring 3 for storeA doesn't block storeB.
func TestAdvisoryLockSlot_IsolatedByStore(t *testing.T) {
	db := testdb.NewDB(t)
	acq := concurrency.NewAdvisoryLockAcquirer(db)
	storeA := uuid.New()
	storeB := uuid.New()

	// Fill all 3 slots for storeA.
	for i := 0; i < 3; i++ {
		_, err := acq.AcquireSlot(context.Background(), storeA)
		require.NoError(t, err)
	}

	// storeB should still be acquirable.
	_, err := acq.AcquireSlot(context.Background(), storeB)
	require.NoError(t, err)
}
