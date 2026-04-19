package concurrency

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type advisoryAcquirer struct{ db *gorm.DB }

// NewAdvisoryLockAcquirer uses pg_try_advisory_lock over N slot keys.
// Single-pod deployments only — each pod has its own Postgres session pool,
// so a lock held on pod A is not visible to pod B. Use only when Redis is
// unavailable and the service is known to be single-replica. Knative
// autoscaling from 0→1→0 is safe; scale-to-2+ with advisory locks risks
// under-counting concurrent sends (acknowledged limitation in the docs).
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
