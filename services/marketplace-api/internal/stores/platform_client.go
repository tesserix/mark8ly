// services/marketplace-api/internal/stores/platform_client.go
package stores

import (
	"context"
	"errors"
)

// ErrPlatformUnavailable signals that platform-api could not be reached.
// StoreMiddleware catches this and falls back to stale cache when possible.
var ErrPlatformUnavailable = errors.New("stores: platform-api unavailable")

// Client is the interface marketplace-api uses to pull store metadata
// from the authoritative platform-api. The real HTTP implementation lands
// in M5. M3 tests inject fakes.
type Client interface {
	GetStore(ctx context.Context, tenantID, storeID string) (*Store, error)
	// GetStoreBySlug looks up a store by its public slug via platform-api's
	// public by-slug route. The public endpoint does NOT return tenant_id,
	// so the returned Store has TenantID == "". Callers on the storefront
	// path must not rely on TenantID from this source — any tenant-scoped
	// work must be performed with the store's own ID. 404 returns (nil, nil).
	GetStoreBySlug(ctx context.Context, slug string) (*Store, error)
}
