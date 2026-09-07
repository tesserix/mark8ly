package public

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// sso_tenant_resolver.go — the production TenantResolver (#820).
//
// The interface has carried a `TODO(wiring)` since it was written, with no
// implementing type anywhere, which is why /sso/:tenantSlug/login could not be
// mounted: the handler answers 503 when TenantResolver is nil.

// StoreSlugLookup is the part of *stores.SlugCache this resolver needs.
// Declared narrowly so the resolver can be tested without a database or a
// platform-api client.
type StoreSlugLookup interface {
	Get(ctx context.Context, slug string) (*stores.Store, error)
}

// StoreSlugTenantResolver resolves the `:tenantSlug` route parameter to a
// tenant id through the stores projection.
//
// The parameter is named tenantSlug, but in this estate the slug in a URL is
// always a STORE slug — it is what `{slug}-admin.mark8ly.com` is built from
// and what platform-api's by-slug endpoint takes. A store belongs to exactly
// one tenant, so the store row is the canonical way to get from a slug to a
// tenant, and this service has no tenants table of its own to consult instead.
//
// It goes through stores.SlugCache rather than the repository directly because
// that cache already solves this exact lookup for admin and storefront
// routing: TTL, singleflight coalescing, a platform-api refresh on miss, and a
// stale-but-cached fallback when platform-api is down. An SSO login failing
// because platform-api is briefly unreachable — when the answer was sitting in
// the projection table — would be a worse outage than the one it reported.
type StoreSlugTenantResolver struct {
	lookup StoreSlugLookup
}

// NewStoreSlugTenantResolver constructs a resolver over the slug cache.
func NewStoreSlugTenantResolver(lookup StoreSlugLookup) *StoreSlugTenantResolver {
	return &StoreSlugTenantResolver{lookup: lookup}
}

// ByTenantSlug returns the tenant that owns the store with this slug.
//
// An unknown slug returns ErrTenantNotFound, which the handler renders as a
// 404 carrying the same body as "this tenant has no SSO config". That merge is
// deliberate: distinguishing "no such tenant" from "that tenant has not set up
// SSO" would let an anonymous caller enumerate which stores exist.
func (r *StoreSlugTenantResolver) ByTenantSlug(ctx context.Context, slug string) (uuid.UUID, error) {
	if r == nil || r.lookup == nil {
		return uuid.Nil, fmt.Errorf("sso: tenant resolver has no store lookup")
	}
	if slug == "" {
		return uuid.Nil, ErrTenantNotFound
	}

	store, err := r.lookup.Get(ctx, slug)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return uuid.Nil, ErrTenantNotFound
		}
		return uuid.Nil, fmt.Errorf("sso: resolve tenant for slug %q: %w", slug, err)
	}
	if store == nil {
		// A nil store with a nil error is not a shape the cache produces
		// today, but the alternative here is a nil dereference inside an
		// auth path, so it is checked rather than assumed.
		return uuid.Nil, ErrTenantNotFound
	}

	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		// The projection holds a tenant id that is not a UUID. That is a data
		// fault, not a missing tenant, and must not be reported as one: a 404
		// would send an operator looking for a store that is right there.
		return uuid.Nil, fmt.Errorf("sso: store %q has an unparseable tenant id %q: %w",
			slug, store.TenantID, err)
	}
	return tenantID, nil
}
