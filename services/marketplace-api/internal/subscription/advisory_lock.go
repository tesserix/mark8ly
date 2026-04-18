// WithAdvisoryLock wraps every subscription-mutating database operation in a
// transaction-scoped pg_advisory_xact_lock keyed on the store's UUID, as
// required by §17.4. Consumers in plangate, state-machine, downgrade-block,
// and webhook dispatch rely on this to serialize concurrent state transitions
// for a single store.
package subscription

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WithAdvisoryLock runs fn inside a transaction that holds a
// pg_advisory_xact_lock on hashtext(store_id). The lock is automatically
// released on commit or rollback. Use for every subscription-mutating code
// path (state transitions, plan changes, downgrade-block re-checks) per §17.4.
func WithAdvisoryLock(ctx context.Context, db *gorm.DB, storeID uuid.UUID, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, storeID.String()).Error; err != nil {
			return fmt.Errorf("subscription: advisory lock for store %s: %w", storeID, err)
		}
		return fn(tx)
	})
}
