//go:build integration

package concurrency_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/campaignbudget/concurrency"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These two tests close the open questions on mark8ly#234 — whether the
// Postgres advisory-lock acquirer is safe to keep as the only implementation
// once the Redis path is deleted.

// TestAdvisoryLockSlot_ReleasedWhenBackendDies is the "released on pod kill"
// question from #234.
//
// The Redis limiter leaned on a 10-minute TTL for this: a pod killed
// mid-request left its slot occupied until the key expired. A session-scoped
// advisory lock is strictly better — it lives on the Postgres backend, so when
// the connection dies the lock is released immediately, with no TTL to wait
// out and no sweeper to run.
//
// pg_terminate_backend is a faithful stand-in for a killed pod: both drop the
// TCP connection and make Postgres tear the backend down.
func TestAdvisoryLockSlot_ReleasedWhenBackendDies(t *testing.T) {
	db := testdb.NewDB(t)
	acq := concurrency.NewAdvisoryLockAcquirer(db)
	storeID := uuid.New()

	// Fill every slot and deliberately DO NOT release — this models a pod
	// holding slots at the moment it is killed.
	for i := 0; i < concurrency.MaxConcurrentSends; i++ {
		_, err := acq.AcquireSlot(context.Background(), storeID)
		require.NoError(t, err)
	}
	_, err := acq.AcquireSlot(context.Background(), storeID)
	require.ErrorIs(t, err, concurrency.ErrTooManyConcurrentSends,
		"slots should be exhausted before the backends are killed")

	// Kill every backend holding an advisory lock except this test's own
	// connection. Safe here because the integration suite runs sequentially
	// (-p 1) against a dedicated test database.
	var killed int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM (
			SELECT pg_terminate_backend(l.pid)
			  FROM pg_locks l
			 WHERE l.locktype = 'advisory'
			   AND l.pid <> pg_backend_pid()
		) AS terminated
	`).Scan(&killed).Error)
	require.Positive(t, killed, "expected to terminate at least one lock-holding backend")

	// With those backends gone the locks are released, so the store is
	// immediately usable again — no TTL wait. That is the whole point:
	// the Redis path would still be refusing this store for 10 minutes.
	//
	// Only ONE re-acquire is asserted, deliberately. Each held slot pins a
	// *sql.Conn for as long as it is held, and killing the backend does NOT
	// hand that connection back to Go's pool — the pool still counts three
	// broken connections as checked out. testdb caps the pool at 4, so
	// asking for three fresh slots here would block on the pool rather than
	// on the lock, and the test would hang instead of failing. One success
	// proves the lock was released; more would only prove the pool size.
	_, err = acq.AcquireSlot(context.Background(), storeID)
	require.NoError(t, err, "the slot should be free once the holding backend died")
}

// TestAdvisoryLockSlot_ConcurrentAcquireGrantsExactlyMax is the honest version
// of #234's "load test the advisory-lock acquirer" checkbox: the existing
// tests acquire sequentially, which never exercises the race the limiter
// exists to prevent.
//
// Run with -race. pg_try_advisory_lock is atomic, so exactly
// MaxConcurrentSends callers must win no matter how many race for a slot.
func TestAdvisoryLockSlot_ConcurrentAcquireGrantsExactlyMax(t *testing.T) {
	db := testdb.NewDB(t)
	acq := concurrency.NewAdvisoryLockAcquirer(db)
	storeID := uuid.New()

	const racers = 24

	var (
		mu       sync.Mutex
		granted  int
		rejected int
		other    []error
		wg       sync.WaitGroup
		start    = make(chan struct{})
	)

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines at once
			_, err := acq.AcquireSlot(context.Background(), storeID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				granted++
			case err == concurrency.ErrTooManyConcurrentSends:
				rejected++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Empty(t, other, "no acquire should fail for a reason other than the cap")
	require.Equal(t, concurrency.MaxConcurrentSends, granted,
		"exactly %d racers must win a slot", concurrency.MaxConcurrentSends)
	require.Equal(t, racers-concurrency.MaxConcurrentSends, rejected,
		"every other racer must be rejected with ErrTooManyConcurrentSends")
}
