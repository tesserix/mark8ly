package concurrency

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type advisoryAcquirer struct{ db *gorm.DB }

// NewAdvisoryLockAcquirer uses pg_try_advisory_lock over N slot keys.
//
// This is CLUSTER-WIDE, not single-pod. An earlier version of this comment
// claimed "single-pod deployments only — a lock held on pod A is not visible
// to pod B", and warned against running more than one replica. That was
// wrong: advisory locks are global to the database, not scoped to a
// connection or a pool, so two sessions can never hold the same key at once
// no matter which replica they belong to. See
// TestAdvisoryLockSlot_LockIsVisibleAcrossConnectionPools, which acquires
// from one pool and asserts a second pool is refused.
//
// Two properties this DOES depend on, both easy to break by accident:
//
//  1. A DIRECT Postgres connection. DATABASE_URL points at
//     mark8ly-postgres-rw. It must NOT be repointed at
//     mark8ly-postgres-pooler-rw, which runs pgbouncer in `transaction` pool
//     mode: a dedicated *sql.Conn pins a connection to pgbouncer, not to a
//     backend, so pg_advisory_unlock could execute on a different backend
//     than the one holding the lock. The unlock would silently return false
//     and the slot would leak until that backend closed — permanently
//     exhausting all MaxConcurrentSends slots for the store.
//
//  2. An uncapped connection pool. Each HELD slot pins one *sql.Conn for as
//     long as it is held, so the ceiling is MaxConcurrentSends × concurrent
//     stores. pkg/db.Open sets no limit (Go's default is unlimited) and
//     Postgres allows 400 connections, so there is ample headroom — but under
//     a small MaxOpenConns the acquirer BLOCKS waiting for a connection
//     instead of returning ErrTooManyConcurrentSends.
//
// Crash behaviour is better than the Redis limiter this replaced: a killed
// pod drops its connection, Postgres tears the backend down, and the slot is
// free immediately, where Redis held it for a 10-minute TTL. See
// TestAdvisoryLockSlot_ReleasedWhenBackendDies.
func NewAdvisoryLockAcquirer(db *gorm.DB) SlotAcquirer {
	return &advisoryAcquirer{db: db}
}

// AcquireSlot tries each of the MaxConcurrentSends advisory lock keys in turn.
// The first successful pg_try_advisory_lock wins the slot. A dedicated database
// connection is checked out per acquire to ensure the session-scoped lock
// survives after the method returns — pg_advisory_unlock in the release closure
// then explicitly releases that connection.
func (a *advisoryAcquirer) AcquireSlot(ctx context.Context, storeID uuid.UUID) (func(), error) {
	sqlDB, err := a.db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("reserve conn for advisory lock: %w", err)
	}

	for slot := 1; slot <= MaxConcurrentSends; slot++ {
		key := fmt.Sprintf("campaign:slot:%s:%d", storeID, slot)
		var acquired bool
		row := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key)
		if err := row.Scan(&acquired); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("try advisory lock slot %d: %w", slot, err)
		}
		if acquired {
			capturedKey := key
			release := func() {
				_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, capturedKey)
				_ = conn.Close()
			}
			return release, nil
		}
	}
	_ = conn.Close()
	return nil, ErrTooManyConcurrentSends
}
