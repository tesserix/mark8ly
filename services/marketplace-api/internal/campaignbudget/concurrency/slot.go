// Package concurrency enforces the max-3 concurrent-send limit per store.
// Two implementations: Redis INCR (preferred, cluster-wide) and a Postgres
// advisory-lock fallback (single-instance, used when Redis is absent).
package concurrency

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// MaxConcurrentSends is the per-store cap on simultaneously active campaign sends.
const MaxConcurrentSends = 3

// ErrTooManyConcurrentSends is returned when the store already has
// MaxConcurrentSends active send jobs.
var ErrTooManyConcurrentSends = errors.New("too many concurrent campaign sends")

// SlotAcquirer abstracts the concurrency backend. Callers depend on this
// interface and main.go wires either the Redis or Postgres implementation.
type SlotAcquirer interface {
	AcquireSlot(ctx context.Context, storeID uuid.UUID) (release func(), err error)
}

// Select returns a Redis-backed acquirer when redisClient is non-nil, else the
// Postgres advisory-lock fallback. Called once from main.go at startup.
func Select(redisClient *redis.Client, db *gorm.DB) SlotAcquirer {
	if redisClient != nil {
		return NewRedisAcquirer(redisClient, 10*time.Minute)
	}
	return NewAdvisoryLockAcquirer(db)
}
