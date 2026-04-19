package concurrency

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type redisAcquirer struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisAcquirer returns a SlotAcquirer backed by Redis INCR+EXPIRE.
// ttl must exceed the longest acceptable send duration — 10 minutes per spec.
// If the pod crashes without calling release, the TTL reclaims the slot.
func NewRedisAcquirer(c *redis.Client, ttl time.Duration) SlotAcquirer {
	return &redisAcquirer{client: c, ttl: ttl}
}

// AcquireSlot atomically increments the per-store send counter and rejects if
// the count exceeds MaxConcurrentSends. The first INCR sets the TTL to protect
// against leaked slots from crashed pods. Returns a release func that decrements.
func (r *redisAcquirer) AcquireSlot(ctx context.Context, storeID uuid.UUID) (func(), error) {
	key := fmt.Sprintf("campaign:slots:%s", storeID)
	n, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis incr slot: %w", err)
	}
	if n == 1 {
		// First holder sets the TTL. Subsequent INCRs don't touch TTL so a
		// stuck first holder is still reclaimed when TTL expires.
		_ = r.client.Expire(ctx, key, r.ttl).Err()
	}
	if n > int64(MaxConcurrentSends) {
		// Decrement on the way out — we incremented, must give it back.
		_ = r.client.Decr(context.Background(), key).Err()
		return nil, ErrTooManyConcurrentSends
	}
	release := func() {
		_ = r.client.Decr(context.Background(), key).Err()
	}
	return release, nil
}
