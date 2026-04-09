// services/marketplace-api/internal/stores/errors.go
package stores

import (
	"errors"
	"time"
)

// ErrNotFound is returned by Repository.GetByIDForTenant when the store
// does not exist OR belongs to a different tenant. Callers must not
// distinguish these cases (no existence leak per spec §13.1.4).
var ErrNotFound = errors.New("stores: not found")

// IsStale reports whether the projection row is older than the TTL.
// The nil guard makes callers' happy-path code shorter.
func IsStale(s *Store, ttl time.Duration) bool {
	if s == nil {
		return true
	}
	return time.Since(s.SyncedAt) > ttl
}
