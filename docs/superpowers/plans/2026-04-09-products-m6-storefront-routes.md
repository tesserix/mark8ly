# Products M6 — Storefront Read Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the four public storefront read endpoints from spec §6.1 / §13.1.2: products list, product by handle, categories list, and products by category slug. All routes are scoped by store slug in the URL path, guarded by a shared-secret header, cacheable via `Cache-Control` + a watermark-derived weak `ETag`, and return a **distinct DTO family** that cannot leak admin-only fields like `cost_price` or `inventory_quantity`. The storefront engine mounts these routes — the admin engine never sees them.

**Architecture:** A new `internal/handlers/storefront` package owns the storefront handler + DTO family + route registrar. A new `internal/stores` slug lookup method (`GetBySlugFresh` with built-in pull-through refresh when stale) replaces the tenant-scoped admin pattern — anonymous storefront traffic has no tenant context, so the middleware reads only the slug and the shared-secret header. The existing `product.Repository.ListPublished` is the **only** repository method the storefront handler can reach; two new thin repo wrappers `GetPublishedByHandle` and `ListPublishedByCategorySlug` add handle-lookup and category-filter forms of the same hard-coded `status='active' AND published_at<=now() AND deleted_at IS NULL` filter. ETag comes from the existing `store_watermarks.products_updated_at` column written by the M3 outbox publisher; the handler issues a 304 on matching `If-None-Match`. A new storefront DTO mapper lives in `internal/handlers/storefront/dto.go` with a compile-time guarantee (field-name reflection test) that it cannot produce the forbidden fields.

**Tech Stack:** Go 1.26, Gin (existing), the M3–M5 packages already in the module. No new external deps.

---

## Status

**Status: ✅ COMPLETE** — all tasks merged to main.

---

## Scope check

Single contained slice inside the existing `services/marketplace-api` Go module. No migrations, no schema changes, no go.work edits. Adds files under `internal/handlers/storefront/`, one new method on `stores.Repository`, two new methods on `product.Repository`, one small addition to `pkg/config`, and a mount in `cmd/marketplace-api/main.go` that was previously a TODO comment (`// Future: storefront route group mounts here in M6.`).

Spec sections authoritative for this milestone:

- §6.1 storefront routes (now superseded by §13.1.2 which re-keys them under `/stores/:storeSlug`)
- §6.3 DTO families — **Storefront** half only; Admin half shipped in M5a/M5b
- §6.4 → §13.1.2 — storefront trust boundary (separate engine + path slug + shared-secret header)
- §8 M6 entry: "Four storefront endpoints live. Distinct DTO family enforced at the type level. `ListPublished` is the only repository method reachable from storefront handlers. Cache headers set. Leak tests assert draft/archived/unpublished products never return, and that `cost_price`/`inventory_quantity` never appear in storefront JSON."
- §13.1.3 — `stores` projection table (the M2 schema; M6 adds a slug-lookup path)
- §13.1.4 — StoreMiddleware pattern (M6 creates a storefront variant — no tenant context)
- §14.1 — `store_watermarks` (M6 reads from it for the ETag)

---

## Decisions locked

1. **Distinct DTO types at the Go package level.** `internal/handlers/storefront/dto.go` defines `StorefrontProductResponse`, `StorefrontVariantResponse`, `StorefrontCategoryRef`, `StorefrontProductOption`, `StorefrontProductOptionValue`, `StorefrontMediaResponse`, `StorefrontPriceRange`. These types are **not** exported from `internal/handlers/admin/` and have no structural compatibility with the Admin* counterparts — the compiler cannot accidentally coerce between them. A regression test uses reflection to scan every storefront field name and assert none of `"cost_price"`, `"cost_price_cents"`, `"inventory_quantity"`, `"cost"`, `"deleted_at"`, `"updated_by"`, `"created_by"`, or `"tenant_id"` appear.
2. **Three defense layers, in order:**
   - Separate Gin engine (already in place via `mode.Storefront` — M6 just mounts on it, never on the admin engine)
   - Path-based store slug lookup (`:storeSlug` URL param, no header trust)
   - Shared-secret `X-Storefront-Key` header, config-gated (empty disables the check for local dev)
3. **`GetBySlugFresh(ctx, slug)` is a dedicated middleware-helper method on `stores.Repository`,** not a replacement for `GetByIDForTenant`. It does its own singleflight-wrapped pull-through: checks the local projection; if missing or stale, calls `platformClient.GetStoreBySlug(ctx, slug)` (a new method on the `stores.Client` interface); upserts and returns. This keeps the storefront path decoupled from the admin path's tenant-scoped lookup — they share zero code at the cache-key level.
4. **Platform client gets a new method.** `stores.Client.GetStoreBySlug(ctx, slug) (*Store, error)` joins `GetStore(ctx, tenantID, storeID)` on the interface. The real `HTTPClient` calls `GET <baseURL>/stores/by-slug/:slug` (platform-api's existing public-store endpoint), and the stub + fake clients return `ErrPlatformUnavailable`. The test double for storefront integration tests uses a pre-seeded map.
5. **ETag is weak, derived from watermark + store id.** Format: `W/"<store_id>-<unix_ms>"`. A client-supplied `If-None-Match` that matches → 304 with empty body. No body-hash ETag; the watermark is authoritative because the outbox publisher bumps it on every mutation. Cheap and deterministic.
6. **Cache-Control is fixed.** `Cache-Control: public, s-maxage=60, stale-while-revalidate=300`. Hard-coded in the handler; no config knob in slice 1. The header is set on every 200 response, NOT on 304 or 404.
7. **`ListPublished` signature gains a `CategorySlug` filter.** Rather than adding a separate `ListPublishedByCategorySlug`, extend the existing `ListPublishedQuery` struct with an optional `CategorySlug string` field and update the repository method to filter when set. This keeps the "only `ListPublished` is reachable" invariant intact — the storefront handler still calls one method with different query params.
8. **`GetPublishedByHandle` is a new repository method, not a reuse of `ListPublished`.** Returns a single `*Aggregate` or `apperrors.NotFound("product")`. The filter is identical to `ListPublished` (status=active, published_at<=now, deleted_at IS NULL) but indexed by `(store_id, handle)` via the existing partial unique index. A regression test asserts the method cannot return a draft.
9. **Category listing for storefront returns only `is_active=true AND deleted_at IS NULL`.** The admin listing (M5b) returns all categories including inactive; storefront strips inactive. A new repository method `ListActiveByStoreID(ctx, storeID) ([]Category, error)` on `category.Repository` — does NOT filter by tenant since the storefront handler has only store scope.
10. **No auth middleware.** Storefront is public. The shared-secret header is the only gate. The shared-secret check is a dedicated `RequireStorefrontKey(secret string) gin.HandlerFunc` — distinct from the admin `HeaderTrustAuth` which requires `X-User-Id`.
11. **Cache headers are set by a helper**, not sprinkled across handlers. `setCacheHeaders(c, store *stores.Store, watermark time.Time)` sets `Cache-Control`, `ETag`, `Last-Modified`. It's called at the end of every 200 response path.
12. **In-stock and low-stock derivation lives in the mapper**, not the storefront DTO itself. `InStock = Variant.InventoryQuantity > 0`, `LowStock = LowStockThreshold != nil && InventoryQuantity <= *LowStockThreshold`. The admin DTO carries the raw numbers; the storefront DTO carries only the booleans.
13. **Price range is computed per product.** `StorefrontPriceRange{Min, Max, CurrencyCode}` where Min/Max are the min/max across all variants' `Price`. If min == max, both fields are equal. The mapper handles this inline.
14. **Storefront tests run against real Postgres via `testdb.NewTx(t)`.** Same pattern as M5a. Leak tests seed a mix of draft/active/archived/soft-deleted products and assert only the right ones come back. The cache-header test seeds a store_watermarks row and asserts the ETag matches.

---

## File structure produced by M6

```
services/marketplace-api/
├── cmd/marketplace-api/main.go                       MODIFIED: construct storefront handler, mount storefront route group
├── pkg/config/config.go                              MODIFIED: add MARKETPLACE_STOREFRONT_KEY
├── pkg/config/config_test.go                         MODIFIED
├── internal/
│   ├── category/
│   │   ├── repository.go                             MODIFIED: add ListActiveByStoreID
│   │   └── repository_integration_test.go            MODIFIED: one new test
│   ├── product/
│   │   ├── repository.go                             MODIFIED: ListPublished gains CategorySlug filter; add GetPublishedByHandle
│   │   └── repository_integration_test.go            MODIFIED: leak regression tests for both methods
│   ├── stores/
│   │   ├── repository.go                             MODIFIED: add GetBySlug + GetBySlugFresh (singleflight pull-through)
│   │   ├── platform_client.go                        MODIFIED: Client interface gains GetStoreBySlug
│   │   ├── platform_http.go                          MODIFIED: HTTPClient implements GetStoreBySlug
│   │   └── repository_integration_test.go            MODIFIED: one new test
│   └── handlers/
│       └── storefront/
│           ├── middleware.go                         NEW: RequireStorefrontKey + StoreContext middleware
│           ├── middleware_test.go                    NEW: unit tests
│           ├── dto.go                                NEW: Storefront* DTO types + ToStorefrontProductResponse/ToStorefrontCategoryRef mappers
│           ├── dto_test.go                           NEW: leak regression (reflect over field names) + mapper unit tests
│           ├── errors.go                             NEW: small render helper (errors here return a different shape — storefront never leaks typed error codes, only 404/500)
│           ├── products.go                           NEW: StorefrontHandler.List, GetByHandle
│           ├── categories.go                         NEW: StorefrontHandler.ListCategories, ListByCategorySlug
│           ├── cache.go                              NEW: setCacheHeaders + checkIfNoneMatch helpers
│           ├── routes.go                             NEW: RegisterStorefront(group, deps)
│           └── storefront_integration_test.go        NEW: API tests including leak assertions + cache header assertions
```

Target sizes: `dto.go` ~200 lines, `products.go` ~150, `categories.go` ~120, `storefront_integration_test.go` ~500. Everything else under 150.

---

## New Go module dependencies

**None.** All imports come from packages already in `go.mod`.

---

## Landmines

- **Landmine #1 (`go.work`):** no new module. `git diff go.work` must be empty per commit.
- **CWD drift:** absolute `cd` on every bash command.
- **(NEW) `stores.Client` interface addition is a breaking change for any test that uses an inline struct as a Client.** The M5a `stubPlatformClient` in main.go and the M5a API test `stubClient` in `products_integration_test.go` both need a new no-op `GetStoreBySlug` method. Grep for implementers of `stores.Client` before you add the method to the interface; update all sites in the same commit.
- **(NEW) Storefront engine must not inherit admin middleware.** The existing `httpserver.New(env, mode.Storefront, log)` constructor returns a separate `*gin.Engine`. Mount the storefront routes on THAT engine, never on the admin engine. In `mode.Both` local dev, mount on the merged router; in `mode.Admin` do NOT mount; in `mode.Storefront` mount.

---

## Task decomposition

10 tasks. Foundation pieces (repository additions + platform client extension) run first; then middleware + DTOs; then handlers; then main.go wiring; then API tests; finally verification.

| # | Task | Approx effort |
|---|---|---|
| 1 | `stores.Client.GetStoreBySlug` + `stores.Repository.GetBySlug/GetBySlugFresh` + tests | 60 min |
| 2 | `category.Repository.ListActiveByStoreID` + test | 30 min |
| 3 | `product.Repository.ListPublished` category filter + `GetPublishedByHandle` + leak tests | 60 min |
| 4 | `internal/handlers/storefront/dto.go` + mapper + leak regression test | 90 min |
| 5 | `internal/handlers/storefront/middleware.go` + `cache.go` + tests | 60 min |
| 6 | `internal/handlers/storefront/products.go` + `categories.go` + `errors.go` + `routes.go` | 90 min |
| 7 | `pkg/config` + `cmd/marketplace-api/main.go` wiring | 45 min |
| 8 | `internal/handlers/storefront/storefront_integration_test.go` — full HTTP stack with leak tests | 2 hours |
| 9 | Update M5a `stubPlatformClient` + `stubClient` for the new interface method | 15 min |
| 10 | Verification + PR | 30 min |
| **Total** | | **~8 hours** |

---

### Task 1: `stores` slug lookup

**Files:**
- Modify: `services/marketplace-api/internal/stores/platform_client.go` — add `GetStoreBySlug(ctx, slug) (*Store, error)` to the `Client` interface
- Modify: `services/marketplace-api/internal/stores/platform_http.go` — add the method to `HTTPClient`, calling `GET <baseURL>/stores/by-slug/:slug` (platform-api has this as a PUBLIC route per `services/platform-api/internal/store/handler.go`)
- Modify: `services/marketplace-api/internal/stores/repository.go` — add `GetBySlug(ctx, slug) (*Store, error)` and `GetBySlugFresh(ctx, slug) (*Store, error)` (the latter does the pull-through refresh when stale or missing)
- Modify: `services/marketplace-api/internal/stores/repository_integration_test.go` — add cases for `GetBySlug` happy-path and cross-store slug collision (none exists due to unique index)
- Modify: `services/marketplace-api/internal/stores/platform_http_test.go` — add 3 new cases: happy path 200, 404, network error

**`GetStoreBySlug` on HTTPClient:**
```go
func (c *HTTPClient) GetStoreBySlug(ctx context.Context, slug string) (*Store, error) {
	// platform-api returns a publicStore envelope (no tenant_id in the response)
	// from its public /stores/by-slug/:slug route. We need the internal fields
	// (tenant_id in particular), so we call the internal route instead:
	// GET /internal/stores/by-slug/:slug. If platform-api doesn't have this
	// route yet, use the list-by-tenant trick — but actually the cleanest path
	// is to use the existing public route since we only need id, slug, name,
	// currency, etc. for the storefront, and the public route DOES expose those.
	//
	// For slice 1 we use the public route. The storefront never needs tenant_id.
	url := fmt.Sprintf("%s/stores/by-slug/%s", c.baseURL, slug)
	// ... standard HTTP dance, decode {"data": publicStore}, return with
	// TenantID left empty (caller doesn't use it in the storefront path).
}
```

Per our earlier inspection, platform-api's `pub.GET("/by-slug/:slug", h.getPublicStoreBySlug)` returns `{"data": publicStore}` where `publicStore` has `id, slug, name, country_code, currency_code, timezone, logo_url, storefront_theme` — no `tenant_id`. That's fine for storefront: `TenantID` in the projection is a nice-to-have but not required by the storefront handler. Document this clearly; `GetStoreBySlug` returns a `*Store` with `TenantID = ""` which the caller must not rely on.

**`GetBySlug` on Repository:**
```go
func (r *gormRepository) GetBySlug(ctx context.Context, slug string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stores: get by slug: %w", err)
	}
	return &s, nil
}
```

**`GetBySlugFresh` is a method on a NEW struct that composes a Repository, a Client, a singleflight.Group, and a TTL.** Not a method on the bare `Repository` — that interface is read-only and doesn't know about refresh. Put it in `internal/stores/slug_cache.go`:

```go
// SlugCache is a pull-through cache for storefront store-slug lookups.
// It wraps a Repository + Client + singleflight to give anonymous
// storefront traffic the same freshness guarantees as the admin path
// has via StoreMiddleware.
type SlugCache struct {
	repo   Repository
	client Client
	flight *singleflight.Group
	ttl    time.Duration
}

func NewSlugCache(repo Repository, client Client, flight *singleflight.Group, ttl time.Duration) *SlugCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &SlugCache{repo: repo, client: client, flight: flight, ttl: ttl}
}

// Get returns the store row for the given slug. On miss or stale, it
// refreshes via the platform client (coalesced through singleflight) and
// upserts the projection table. Returns ErrNotFound when the store
// doesn't exist in either place.
func (c *SlugCache) Get(ctx context.Context, slug string) (*Store, error) {
	cached, err := c.repo.GetBySlug(ctx, slug)
	if err == nil && !IsStale(cached, c.ttl) {
		return cached, nil
	}
	result, refreshErr, _ := c.flight.Do("slug:"+slug, func() (interface{}, error) {
		fresh, ferr := c.client.GetStoreBySlug(ctx, slug)
		if ferr != nil {
			return nil, ferr
		}
		if fresh == nil {
			return nil, ErrNotFound
		}
		fresh.SyncedAt = time.Now()
		if err := c.repo.Upsert(ctx, fresh); err != nil {
			return nil, err
		}
		return fresh, nil
	})
	if refreshErr == nil && result != nil {
		return result.(*Store), nil
	}
	// Stale-but-present fallback — same 24h ceiling as admin StoreMiddleware
	if err == nil && cached != nil && time.Since(cached.SyncedAt) < 24*time.Hour {
		return cached, nil
	}
	return nil, ErrNotFound
}
```

Commit: `feat(marketplace-api): add stores slug lookup + SlugCache pull-through (M6)`.

---

### Task 2: `category.Repository.ListActiveByStoreID`

**Files:**
- Modify: `services/marketplace-api/internal/category/repository.go`
- Modify: `services/marketplace-api/internal/category/repository_integration_test.go`

**Method:**
```go
// ListActiveByStoreID returns active (is_active=true) non-deleted
// categories for a store, ordered by position, name. Scoped by store_id
// only — storefront traffic has no tenant context.
func (r *gormRepository) ListActiveByStoreID(ctx context.Context, storeID string) ([]Category, error) {
	var cats []Category
	if err := r.db.WithContext(ctx).
		Where("store_id = ? AND is_active = true AND deleted_at IS NULL", storeID).
		Order("position ASC, name ASC").
		Find(&cats).Error; err != nil {
		return nil, fmt.Errorf("category: list active by store: %w", err)
	}
	return cats, nil
}
```

Add to the `Repository` interface at the top of the file.

**Test case:** Seed 4 categories — 2 active non-deleted, 1 inactive, 1 soft-deleted. Call the method. Assert 2 results.

Commit: `feat(marketplace-api): add category.Repository.ListActiveByStoreID (M6)`.

---

### Task 3: `product.Repository` — `CategorySlug` filter + `GetPublishedByHandle`

**Files:**
- Modify: `services/marketplace-api/internal/product/repository.go`
- Modify: `services/marketplace-api/internal/product/repository_integration_test.go`

**Changes to `ListPublishedQuery`:**
```go
type ListPublishedQuery struct {
	StoreID        string
	CategorySlug   string // optional filter; if set, only products linked to that category (by slug, not ID)
	Page, PageSize int
}
```

**Changes to `ListPublished`:** add the category join when `CategorySlug` is non-empty. Use `INNER JOIN product_categories pc ON pc.product_id = p.id INNER JOIN categories c ON c.id = pc.category_id WHERE c.store_id = ? AND c.slug = ? AND c.deleted_at IS NULL AND c.is_active = true`. Without the filter, keep the existing query.

**New method `GetPublishedByHandle`:**
```go
// GetPublishedByHandle returns a single published product by (store, handle).
// The filter is hard-coded identical to ListPublished: status='active'
// AND published_at<=now() AND deleted_at IS NULL. There is no way to
// return a draft or archived row through this method.
func (r *gormRepository) GetPublishedByHandle(ctx context.Context, storeID, handle string) (*Aggregate, error) {
	var prod Product
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND handle = ? AND status = 'active' AND deleted_at IS NULL AND published_at IS NOT NULL AND published_at <= now()",
			storeID, handle).
		First(&prod).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("product")
	}
	if err != nil {
		return nil, fmt.Errorf("product: get published by handle: %w", err)
	}
	// Load the full aggregate via a helper that calls the existing preload path.
	return r.loadAggregate(ctx, &prod)
}
```

Factor out a `loadAggregate(ctx, *Product) (*Aggregate, error)` helper that the existing `GetByIDForStore` and the new `GetPublishedByHandle` share — avoids duplicating the preload chain.

Add both to the `Repository` interface.

**Leak tests** (new):
1. `TestIntegration_ProductRepo_ListPublished_ExcludesDraft_Archived_Unpublished_SoftDeleted` — seed 5 products: active+published, draft, archived, active+future-published, active+soft-deleted. Call `ListPublished`. Assert only the first is returned.
2. `TestIntegration_ProductRepo_ListPublished_WithCategorySlug_Filters` — seed 3 products each linked to different categories. Query with `CategorySlug: "apparel"`. Assert only the product in "apparel" is returned.
3. `TestIntegration_ProductRepo_GetPublishedByHandle_ActivePublished_Returns` — seed active+published. Assert returned.
4. `TestIntegration_ProductRepo_GetPublishedByHandle_Draft_Returns404` — seed draft product with the handle. Assert `apperrors.ErrNotFound`.
5. `TestIntegration_ProductRepo_GetPublishedByHandle_SoftDeleted_Returns404`.
6. `TestIntegration_ProductRepo_GetPublishedByHandle_CrossStore_Returns404` — handle exists in store A, query against store B.

Commit: `feat(marketplace-api): extend product.Repository with CategorySlug filter + GetPublishedByHandle + leak tests (M6)`.

---

### Task 4: `internal/handlers/storefront/dto.go` + mapper + leak regression

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/dto.go`
- Create: `services/marketplace-api/internal/handlers/storefront/dto_test.go`

**DTO types** (per spec §6.3):
```go
type StorefrontProductResponse struct {
	ID             string                      `json:"id"`
	Handle         string                      `json:"handle"`
	Title          string                      `json:"title"`
	Description    *string                     `json:"description,omitempty"`
	Tags           []string                    `json:"tags"`
	SEOTitle       *string                     `json:"seo_title,omitempty"`
	SEODescription *string                     `json:"seo_description,omitempty"`
	Categories     []StorefrontCategoryRef     `json:"categories"`
	Options        []StorefrontProductOption   `json:"options"`
	Variants       []StorefrontVariantResponse `json:"variants"`
	Media          []StorefrontMediaResponse   `json:"media"`
	PriceRange     StorefrontPriceRange        `json:"price_range"`
	PublishedAt    time.Time                   `json:"published_at"`
}

type StorefrontProductOption struct {
	Name   string                          `json:"name"`
	Values []StorefrontProductOptionValue  `json:"values"`
}

type StorefrontProductOptionValue struct {
	Value    string `json:"value"`
	Position int    `json:"position"`
}

type StorefrontVariantResponse struct {
	ID             string                  `json:"id"`
	Price          decimal.Decimal         `json:"price"`
	CompareAtPrice *decimal.Decimal        `json:"compare_at_price,omitempty"`
	CurrencyCode   string                  `json:"currency_code"`
	InStock        bool                    `json:"in_stock"`
	LowStock       bool                    `json:"low_stock"`
	OptionValues   []StorefrontVariantOptionRef `json:"option_values"`
	// Note: NO InventoryQuantity, NO CostPrice, NO audit fields.
}

type StorefrontVariantOptionRef struct {
	OptionName string `json:"option_name"`
	Value      string `json:"value"`
}

type StorefrontMediaResponse struct {
	URL       string  `json:"url"`
	Alt       *string `json:"alt,omitempty"`
	Position  int     `json:"position"`
	MediaType string  `json:"media_type"`
	Width     *int    `json:"width,omitempty"`
	Height    *int    `json:"height,omitempty"`
}

type StorefrontCategoryRef struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type StorefrontCategoryResponse struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
}

type StorefrontPriceRange struct {
	Min          decimal.Decimal `json:"min"`
	Max          decimal.Decimal `json:"max"`
	CurrencyCode string          `json:"currency_code"`
}
```

**Mapper**: `ToStorefrontProductResponse(a *product.Aggregate, categories []StorefrontCategoryRef) StorefrontProductResponse`. Computes `PriceRange` by scanning variants' `Price`. Computes per-variant `InStock` = `InventoryQuantity > 0`, `LowStock` = `LowStockThreshold != nil && InventoryQuantity <= *LowStockThreshold`.

**Leak regression test** (the core M6 guarantee):

```go
func TestStorefrontDTO_NoLeakFields(t *testing.T) {
	forbidden := []string{
		"cost_price", "cost_price_cents", "cost",
		"inventory_quantity", "stock_quantity",
		"deleted_at", "updated_by", "created_by",
		"tenant_id",
	}
	types := []reflect.Type{
		reflect.TypeOf(storefront.StorefrontProductResponse{}),
		reflect.TypeOf(storefront.StorefrontVariantResponse{}),
		reflect.TypeOf(storefront.StorefrontCategoryRef{}),
		reflect.TypeOf(storefront.StorefrontCategoryResponse{}),
		reflect.TypeOf(storefront.StorefrontMediaResponse{}),
		reflect.TypeOf(storefront.StorefrontProductOption{}),
		reflect.TypeOf(storefront.StorefrontProductOptionValue{}),
		reflect.TypeOf(storefront.StorefrontVariantOptionRef{}),
		reflect.TypeOf(storefront.StorefrontPriceRange{}),
	}
	for _, tp := range types {
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			// Check the JSON tag (snake_case) against the forbidden set.
			tag := f.Tag.Get("json")
			name := strings.SplitN(tag, ",", 2)[0]
			for _, bad := range forbidden {
				if name == bad {
					t.Errorf("%s.%s has forbidden json tag %q",
						tp.Name(), f.Name, name)
				}
				// Also check the Go field name as a backstop.
				if strings.EqualFold(f.Name, bad) {
					t.Errorf("%s.%s has forbidden field name %q",
						tp.Name(), f.Name, f.Name)
				}
			}
		}
	}
}
```

Plus mapper unit tests:
- `TestToStorefrontProductResponse_PriceRange_MinMax` — 3 variants with prices 10, 20, 15 → min=10 max=20.
- `TestToStorefrontProductResponse_PriceRange_SinglePrice` — 1 variant → min=max.
- `TestToStorefrontProductResponse_InStock_PositiveQty_True` — variant with qty=5 → true.
- `TestToStorefrontProductResponse_InStock_ZeroQty_False`.
- `TestToStorefrontProductResponse_LowStock_BelowThreshold_True` — qty=2, threshold=5 → true.
- `TestToStorefrontProductResponse_LowStock_NoThreshold_False`.
- `TestToStorefrontProductResponse_OmitsAuditFields` — marshal to JSON, assert output does not contain `"cost_price"`, `"inventory_quantity"`, `"deleted_at"`.

Commit: `feat(marketplace-api): add storefront DTO family + leak regression test (M6)`.

---

### Task 5: `middleware.go` + `cache.go`

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/middleware.go`
- Create: `services/marketplace-api/internal/handlers/storefront/cache.go`
- Create: `services/marketplace-api/internal/handlers/storefront/middleware_test.go`

**`middleware.go`** — two middlewares:

```go
package storefront

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// RequireStorefrontKey returns a middleware that rejects requests
// missing or mismatching X-Storefront-Key. When secret is empty the
// middleware is a no-op — used for local dev and tests.
func RequireStorefrontKey(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		if c.GetHeader("X-Storefront-Key") != secret {
			respondNotFound(c)
			return
		}
		c.Next()
	}
}

// StoreContext resolves the :storeSlug path param to a store row via the
// SlugCache. Sets the store on the gin context as "store". Returns 404
// on miss (no existence leak).
func StoreContext(cache *stores.SlugCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("storeSlug")
		if slug == "" {
			respondNotFound(c)
			return
		}
		store, err := cache.Get(c.Request.Context(), slug)
		if err != nil {
			if !errors.Is(err, stores.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
					"error": "internal", "message": "internal server error",
				})
				return
			}
			respondNotFound(c)
			return
		}
		// Reject stores that are not active.
		if store.Status != stores.StatusActive {
			respondNotFound(c)
			return
		}
		c.Set("store", store)
		c.Next()
	}
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "not found",
	})
}
```

**`cache.go`** — ETag + Cache-Control helpers:

```go
package storefront

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

const (
	cacheControlValue = "public, s-maxage=60, stale-while-revalidate=300"
)

// buildETag returns the weak ETag for a store's current products watermark.
// Format: W/"<store_id>-<unix_ms>"
func buildETag(store *stores.Store, watermark time.Time) string {
	return fmt.Sprintf(`W/"%s-%d"`, store.ID, watermark.UnixMilli())
}

// setCacheHeaders writes Cache-Control, ETag, and Last-Modified on a
// successful response. Must be called BEFORE c.JSON.
func setCacheHeaders(c *gin.Context, store *stores.Store, watermark time.Time) {
	c.Header("Cache-Control", cacheControlValue)
	c.Header("ETag", buildETag(store, watermark))
	c.Header("Last-Modified", watermark.UTC().Format(http.TimeFormat))
	// Vary: Accept-Encoding is already added by Gin's compression middleware
	// if enabled; we add it explicitly to be safe.
	c.Header("Vary", "Accept-Encoding, X-Storefront-Key")
}

// checkIfNoneMatch returns true and writes 304 if the client's
// If-None-Match header matches the current ETag. The handler short-
// circuits when this returns true.
func checkIfNoneMatch(c *gin.Context, store *stores.Store, watermark time.Time) bool {
	wantETag := buildETag(store, watermark)
	if c.GetHeader("If-None-Match") == wantETag {
		c.Header("Cache-Control", cacheControlValue)
		c.Header("ETag", wantETag)
		c.Status(http.StatusNotModified)
		return true
	}
	return false
}
```

Note: `import "net/http"` in cache.go.

**Middleware tests:**
1. `TestRequireStorefrontKey_EmptySecret_Passthrough` — secret empty, no header → Next called.
2. `TestRequireStorefrontKey_MissingHeader_404` — secret set, no header → 404.
3. `TestRequireStorefrontKey_MismatchedHeader_404` — secret="abc", header="xyz" → 404.
4. `TestRequireStorefrontKey_MatchingHeader_Passthrough` — secret="abc", header="abc" → Next.
5. `TestStoreContext_ActiveStore_SetsContext` — fake SlugCache returns an active store → Next called, `c.Get("store")` non-nil.
6. `TestStoreContext_MissingStore_404` — fake returns ErrNotFound → 404.
7. `TestStoreContext_SuspendedStore_404` — fake returns a store with Status=suspended → 404.
8. `TestStoreContext_CacheError_500` — fake returns a non-NotFound error → 500.

Commit: `feat(marketplace-api): add storefront middleware + cache helpers (M6)`.

---

### Task 6: Storefront handlers + routes

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/errors.go` — tiny render helper returning generic `{"error":"not_found","message":"not found"}` or `{"error":"internal","message":"internal server error"}`. Never exposes typed error codes to anonymous traffic.
- Create: `services/marketplace-api/internal/handlers/storefront/products.go` — `StorefrontHandler.List`, `GetByHandle`
- Create: `services/marketplace-api/internal/handlers/storefront/categories.go` — `ListCategories`, `ListByCategorySlug`
- Create: `services/marketplace-api/internal/handlers/storefront/routes.go`

**Handler struct:**
```go
type StorefrontHandler struct {
	productRepo  product.Repository
	categoryRepo category.Repository
	watermarks   stores.WatermarkReader // new interface — see below
	logger       *slog.Logger
}

func NewStorefrontHandler(
	productRepo product.Repository,
	categoryRepo category.Repository,
	watermarks stores.WatermarkReader,
	logger *slog.Logger,
) *StorefrontHandler
```

**`stores.WatermarkReader`** — new small interface in `internal/stores/repository.go`:

```go
// WatermarkReader returns the current products_updated_at watermark for
// a store. Used by storefront handlers to compute ETags.
type WatermarkReader interface {
	GetProductsWatermark(ctx context.Context, storeID string) (time.Time, error)
}

// Add the method to gormRepository:
func (r *gormRepository) GetProductsWatermark(ctx context.Context, storeID string) (time.Time, error) {
	var row StoreWatermark
	err := r.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// No watermark row yet — either the store has no mutations OR
		// the publisher hasn't written one. Fall back to a safe default
		// (unix epoch) so the ETag is still deterministic; the client
		// will get stale data for up to s-maxage seconds, which is
		// acceptable for an empty store.
		return time.Unix(0, 0), nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("stores: get watermark: %w", err)
	}
	return row.ProductsUpdatedAt, nil
}
```

Add this to Task 1's commit (it's part of the stores package changes) OR ship it in Task 6 — reviewer preference. Plan commits it in Task 1 to keep all stores changes together.

Actually — re-locate to Task 1 for cohesion. Update Task 1's file list accordingly.

**`products.go`:**
```go
// List handles GET /storefront/stores/:storeSlug/products
func (h *StorefrontHandler) List(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}

	var q struct {
		Page     int `form:"page" binding:"omitempty,min=1"`
		PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
	}
	if err := c.ShouldBindQuery(&q); err != nil {
		respondNotFound(c) // storefront never leaks validation error text
		return
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}

	aggs, err := h.productRepo.ListPublished(c.Request.Context(), product.ListPublishedQuery{
		StoreID: store.ID, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]StorefrontProductResponse, 0, len(aggs))
	for i := range aggs {
		catRefs := h.resolveStorefrontCategoryRefs(c, &aggs[i], store.ID)
		out = append(out, ToStorefrontProductResponse(&aggs[i], catRefs))
	}

	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"page": q.Page, "page_size": q.PageSize},
	})
}

// GetByHandle handles GET /storefront/stores/:storeSlug/products/:handle
func (h *StorefrontHandler) GetByHandle(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	handle := c.Param("handle")
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}
	agg, err := h.productRepo.GetPublishedByHandle(c.Request.Context(), store.ID, handle)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			respondNotFound(c)
			return
		}
		respondInternal(c, h.logger, err)
		return
	}
	catRefs := h.resolveStorefrontCategoryRefs(c, agg, store.ID)
	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, ToStorefrontProductResponse(agg, catRefs))
}
```

`resolveStorefrontCategoryRefs` hydrates `StorefrontCategoryRef` values from the product's category IDs — same pattern as admin's helper but with storefront types.

**`categories.go`:**
```go
// ListCategories handles GET /storefront/stores/:storeSlug/categories
func (h *StorefrontHandler) ListCategories(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}
	cats, err := h.categoryRepo.ListActiveByStoreID(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	out := make([]StorefrontCategoryResponse, 0, len(cats))
	for _, cat := range cats {
		out = append(out, StorefrontCategoryResponse{
			Name: cat.Name, Slug: cat.Slug, Position: cat.Position,
		})
	}
	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// ListByCategorySlug handles GET /storefront/stores/:storeSlug/categories/:slug/products
func (h *StorefrontHandler) ListByCategorySlug(c *gin.Context) {
	store := c.MustGet("store").(*stores.Store)
	slug := c.Param("slug")
	watermark, err := h.watermarks.GetProductsWatermark(c.Request.Context(), store.ID)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if checkIfNoneMatch(c, store, watermark) {
		return
	}

	var q struct {
		Page     int `form:"page" binding:"omitempty,min=1"`
		PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
	}
	if err := c.ShouldBindQuery(&q); err != nil {
		respondNotFound(c)
		return
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}

	aggs, err := h.productRepo.ListPublished(c.Request.Context(), product.ListPublishedQuery{
		StoreID:      store.ID,
		CategorySlug: slug,
		Page:         q.Page,
		PageSize:     q.PageSize,
	})
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]StorefrontProductResponse, 0, len(aggs))
	for i := range aggs {
		catRefs := h.resolveStorefrontCategoryRefs(c, &aggs[i], store.ID)
		out = append(out, ToStorefrontProductResponse(&aggs[i], catRefs))
	}

	setCacheHeaders(c, store, watermark)
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"page": q.Page, "page_size": q.PageSize},
	})
}
```

**`routes.go`:**
```go
type Deps struct {
	Handler        *StorefrontHandler
	SlugCache      *stores.SlugCache
	StorefrontKey  string
}

// RegisterStorefront mounts the 4 storefront routes on the given router group.
// Chain: RequireStorefrontKey → StoreContext → handler. No auth, no authz,
// no admin middleware.
func RegisterStorefront(router *gin.RouterGroup, deps Deps) {
	keyMW := RequireStorefrontKey(deps.StorefrontKey)
	storeMW := StoreContext(deps.SlugCache)

	storefront := router.Group("/storefront/stores/:storeSlug", keyMW, storeMW)
	{
		storefront.GET("/products", deps.Handler.List)
		storefront.GET("/products/:handle", deps.Handler.GetByHandle)
		storefront.GET("/categories", deps.Handler.ListCategories)
		storefront.GET("/categories/:slug/products", deps.Handler.ListByCategorySlug)
	}
}
```

Commit: `feat(marketplace-api): add storefront handlers + routes (M6)`.

---

### Task 7: `pkg/config` + `cmd/marketplace-api/main.go` wiring

**Config change:**
```go
StorefrontKey string `envconfig:"MARKETPLACE_STOREFRONT_KEY" default:""`
```

**main.go changes:**

1. After the existing admin wiring block, add a storefront wiring block guarded by `m == mode.Storefront || m == mode.Both`:

```go
var storefrontDeps storefront.Deps
if m == mode.Storefront || m == mode.Both {
	productRepo := product.NewRepository(conn)      // safe to re-construct
	categoryRepo := category.NewRepository(conn)
	storesRepo := stores.NewRepository(conn)
	slugFlight := &singleflight.Group{}
	var storefrontPlatformClient stores.Client
	if cfg.PlatformAPIURL != "" {
		storefrontPlatformClient = stores.NewHTTPClient(cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil)
	} else {
		storefrontPlatformClient = stubPlatformClient{}
	}
	slugCache := stores.NewSlugCache(storesRepo, storefrontPlatformClient, slugFlight, 5*time.Minute)
	storefrontHandler := storefront.NewStorefrontHandler(productRepo, categoryRepo, storesRepo, log)
	storefrontDeps = storefront.Deps{
		Handler:       storefrontHandler,
		SlugCache:     slugCache,
		StorefrontKey: cfg.StorefrontKey,
	}
}
```

2. Mount the routes in the mode switch:
   - `mode.Both`: `storefront.RegisterStorefront(r.Group("/api/v1"), storefrontDeps)` — replaces the existing `// Future: storefront route group mounts here in M6.` TODO comment.
   - `mode.Storefront`: `storefront.RegisterStorefront(engine.Group("/api/v1"), storefrontDeps)`.
   - `mode.Admin`: do NOT mount.

3. Add imports: `"github.com/mark8ly/marketplace-api/internal/handlers/storefront"`.

Commit: `feat(marketplace-api): wire storefront route group + SlugCache in main (M6)`.

---

### Task 8: Storefront API integration tests

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/storefront_integration_test.go`

Build tag `//go:build integration`. Package `storefront_test`. Uses `testdb.NewDB(t, tables...)` for real commits (some tests seed the stores projection + watermarks + products across multiple commits).

**Helpers:**
- `setupStorefrontRouter(t, db)` — constructs a real router with all dependencies wired + the SlugCache backed by an in-memory fake client. Returns `(*gin.Engine, fakeSlugClient)`.
- `seedStorefrontStore(t, db, slug, currency)` — inserts a `stores` projection row with the given slug, returns `(storeID, tenantID)`.
- `seedStorefrontWatermark(t, db, storeID, ts)` — inserts or updates the watermark row.
- `seedProduct(t, db, svc, storeID, tenantID, title, status)` — uses the M3 product service to create a product with a single variant. Returns the aggregate.

**Cases (18+):**

**Happy path:**
1. `TestAPI_Storefront_ListProducts_HappyPath_200` — seed store + 3 active published products. GET list. Assert 200, 3 results, DTO fields match.
2. `TestAPI_Storefront_GetProductByHandle_200` — seed product with handle "linen-shirt". GET by handle. Assert 200.
3. `TestAPI_Storefront_ListCategories_HappyPath_200` — seed 2 active categories. GET. 2 results.
4. `TestAPI_Storefront_ListByCategorySlug_200` — seed product linked to "apparel". GET by slug. Returns that product.

**Leak regression (the main point of M6):**
5. `TestAPI_Storefront_ListProducts_ExcludesDraft` — seed 1 active + 2 drafts. GET list. Only 1 returned.
6. `TestAPI_Storefront_ListProducts_ExcludesArchived` — seed 1 active + 2 archived. Only 1.
7. `TestAPI_Storefront_ListProducts_ExcludesSoftDeleted` — seed 1 active + 1 soft-deleted. Only 1.
8. `TestAPI_Storefront_ListProducts_ExcludesUnpublished` — seed 1 active + 1 with future `published_at`. Only 1.
9. `TestAPI_Storefront_ListProducts_JSONHasNoCostPrice` — seed product with cost_price set. GET list. `strings.Contains(responseBody, "cost_price")` must be false.
10. `TestAPI_Storefront_ListProducts_JSONHasNoInventoryQuantity` — same, assert `"inventory_quantity"` absent.
11. `TestAPI_Storefront_ListProducts_JSONHasNoTenantID` — assert `"tenant_id"` absent.
12. `TestAPI_Storefront_ListProducts_JSONHasNoDeletedAt`.
13. `TestAPI_Storefront_GetByHandle_Draft_Returns404` — seed draft with handle, GET → 404.
14. `TestAPI_Storefront_GetByHandle_SoftDeleted_Returns404`.
15. `TestAPI_Storefront_GetByHandle_CrossStore_Returns404` — handle exists in store A, request against store B's slug → 404.

**Store slug + auth:**
16. `TestAPI_Storefront_UnknownStoreSlug_404` — request with a slug that doesn't exist → 404.
17. `TestAPI_Storefront_SuspendedStore_404` — seed store with status=suspended → 404.
18. `TestAPI_Storefront_MissingKey_404` — router built with secret="prod-secret", request without header → 404.
19. `TestAPI_Storefront_WrongKey_404` — request with wrong header value → 404.
20. `TestAPI_Storefront_CorrectKey_Passes` — header matches → 200.
21. `TestAPI_Storefront_EmptySecret_NoCheck` — router built with secret="" → request without header → 200.

**Cache headers:**
22. `TestAPI_Storefront_ListProducts_SetsCacheControl` — assert `Cache-Control: public, s-maxage=60, stale-while-revalidate=300`.
23. `TestAPI_Storefront_ListProducts_SetsETagFromWatermark` — seed a known watermark; assert the ETag value contains `W/"<store_id>-<unix_ms>"` with the right unix_ms.
24. `TestAPI_Storefront_ListProducts_IfNoneMatch_Returns304` — first request captures ETag; second request with `If-None-Match: <etag>` returns 304 with no body.
25. `TestAPI_Storefront_ListProducts_IfNoneMatch_StaleETag_Returns200` — If-None-Match with a wrong ETag returns 200 + full body.

Commit: `test(marketplace-api): storefront API integration tests with leak assertions (M6)`.

---

### Task 9: Update existing `stores.Client` implementers

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` — add `GetStoreBySlug` method to the existing `stubPlatformClient` (returns `nil, stores.ErrPlatformUnavailable`)
- Modify: `services/marketplace-api/internal/handlers/admin/products_integration_test.go` — find the `stubClient` (or whatever the M5a test named its fake) and add a no-op `GetStoreBySlug` method

**Scope:** Every type that satisfies `stores.Client` must gain the new method or the build breaks. `grep -rn "func.*GetStore(ctx.*stores\.Store" services/marketplace-api/ | grep -v _test` finds the production impls; `grep -rn "stores.Client" services/marketplace-api/` finds all references.

The changes are 4–6 lines total across all sites.

Commit: `fix(marketplace-api): add GetStoreBySlug no-op to existing Client implementers (M6)`.

This task could be merged into Task 1's commit (where the interface method is added) — preferred. If the subagent finds it simpler to do as a separate commit, that's also fine.

---

### Task 10: Verification + PR

- [x] **Step 1: Full run**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go vet ./... && go vet -tags=integration ./... && go build ./... && go test ./... -race && go test -tags integration ./... -race
```

All clean. Integration tests skip cleanly without TEST_DATABASE_URL.

- [x] **Step 2: Verify branch scope**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && git diff --stat main..feat/products-m6-storefront-routes
```

Only files under `services/marketplace-api/{internal/{stores,category,product,handlers/storefront,handlers/admin},cmd/marketplace-api,pkg/config}`, `services/marketplace-api/go.mod` (untouched — no new deps), and the M6 plan doc.

- [x] **Step 3: Push**

```
git push -u origin feat/products-m6-storefront-routes
```

- [x] **Step 4: Open PR**

Title: `feat(marketplace-api): products M6 — storefront read routes with leak regression`

Body covers:
- Summary: 4 routes, distinct DTO family, leak regression test, cache headers + ETag, shared-secret gate.
- Permission/trust model: no auth, path slug, shared secret, separate engine.
- What's NOT in this PR: admin routes (already shipped in M5), signed ETag variants, CDN purging hooks.
- Test plan: leak assertions list (cost_price, inventory_quantity, tenant_id, deleted_at absent); cache header assertions; 304 flow verified.

- [x] **Step 5: Merge.**

---

## Exit criteria

- [x] 4 storefront routes live under `/api/v1/storefront/stores/:storeSlug/...`
- [x] Separate Gin engine mounts them (storefront mode); admin mode NEVER mounts them
- [x] `Cache-Control: public, s-maxage=60, stale-while-revalidate=300` on every 200
- [x] Weak ETag derived from `store_watermarks.products_updated_at` + store id
- [x] `If-None-Match` matching current ETag returns 304
- [x] Storefront DTO family contains zero leak fields (reflect-based regression test)
- [x] Draft / archived / soft-deleted / unpublished products NEVER appear in any storefront response
- [x] Handle lookup (`GetPublishedByHandle`) cannot return a non-published product
- [x] Category lookup returns only `is_active=true AND deleted_at IS NULL`
- [x] Unknown store slug → 404 (no existence leak)
- [x] Suspended store → 404
- [x] `X-Storefront-Key` check blocks wrong/missing keys with 404 (not 401 — no leak)
- [x] `go.mod` untouched
- [x] `go.work` untouched
- [x] PR is open and CI is green

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. stores slug lookup + platform client + SlugCache + WatermarkReader | 60 min |
| 2. category.ListActiveByStoreID | 30 min |
| 3. product.ListPublished category filter + GetPublishedByHandle + leak tests | 60 min |
| 4. storefront DTO family + mapper + leak regression | 90 min |
| 5. middleware + cache helpers + tests | 60 min |
| 6. handlers + routes | 90 min |
| 7. config + main.go | 45 min |
| 8. API integration tests | 2 hours |
| 9. Update existing Client implementers | 15 min (or folded into Task 1) |
| 10. Verification + PR | 30 min |
| **Total** | **~8 hours** |

Smaller than M5a/M5b because no external integrations (GCS, platform HTTP) and no authz wiring — just read paths + DTOs + cache headers + leak tests.
