// Package concurrency enforces the max-3 concurrent-send limit per store,
// using a Postgres session-scoped advisory lock.
//
// It used to carry a second, Redis-backed implementation chosen by Select().
// That path was already unreachable — cmd/marketplace-api passed a hardcoded
// nil Redis client and no deployment ever set a Redis URL — so mark8ly#234
// removed it rather than keep a limiter nobody could reach.
//
// Advisory locks are cluster-wide, not per-pod: every marketplace-api replica
// shares one Postgres, so a lock taken by one pod is visible to all of them.
// (The old package comment called this path "single-instance", which was
// wrong.) They also beat the Redis path on crash recovery: a killed pod drops
// its connection, Postgres tears the backend down, and the slot frees
// immediately — where Redis held it for a 10-minute TTL.
package concurrency

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// MaxConcurrentSends is the per-store cap on simultaneously active campaign sends.
const MaxConcurrentSends = 3

// ErrTooManyConcurrentSends is returned when the store already has
// MaxConcurrentSends active send jobs.
var ErrTooManyConcurrentSends = errors.New("too many concurrent campaign sends")

// SlotAcquirer abstracts the concurrency backend so handlers depend on an
// interface rather than on the advisory-lock implementation.
type SlotAcquirer interface {
	AcquireSlot(ctx context.Context, storeID uuid.UUID) (release func(), err error)
}
