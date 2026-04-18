//go:build integration

package subscription_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestWithAdvisoryLock_Serializes verifies that two goroutines competing for
// the same store's advisory lock are strictly serialized. Peak concurrency
// inside the critical section must be exactly 1.
func TestWithAdvisoryLock_Serializes(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	storeID := uuid.New()

	var inside int32
	var peakInside int32
	var wg sync.WaitGroup

	run := func() {
		defer wg.Done()
		err := subscription.WithAdvisoryLock(context.Background(), db, storeID, func(tx *gorm.DB) error {
			n := atomic.AddInt32(&inside, 1)
			for {
				p := atomic.LoadInt32(&peakInside)
				if n > p {
					if atomic.CompareAndSwapInt32(&peakInside, p, n) {
						break
					}
					continue
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&inside, -1)
			return nil
		})
		require.NoError(t, err)
	}

	wg.Add(2)
	go run()
	go run()
	wg.Wait()

	require.EqualValues(t, 1, peakInside, "locks must serialize — peak concurrency inside critical section must be 1")
}
