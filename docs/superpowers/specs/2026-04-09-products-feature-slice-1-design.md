---
title: Products feature — slice 1 design
date: 2026-04-09
status: draft
---

# Products feature — slice 1 design

First production feature on top of the rewritten Mark8ly foundation. Introduces the consolidated `marketplace-api` service, a products module (with categories and media as sibling modules), full OpenFGA-enforced admin CRUD, public storefront read routes, and an editorial admin UX built on `@tesserix/web` + `@repo/ui`.

---

## 0 · Context & motivation

- **Foundation status.** Onboarding, admin login, storefront shell, and team invites are working. Products is the first real commerce feature.
- **Microservice consolidation.** Per `docs/planning/01-architecture-decisions.md`, all `mp-*` services collapse into a single `marketplace-api` binary sharing one Postgres database and one transaction boundary. Products is the first module inside that service, which doesn't exist yet — this slice creates it.
- **Brand direction.** Paper · Ink · Moss editorial system (`@repo/ui/styles/mark8ly-tokens.css`). Calm, confident, no dev required. The product feature must feel closer to a thoughtful magazine than a dashboard.
- **Rewrite opportunity.** The legacy products service had real schema smells (money as VARCHAR, stock on both product and variant, flat variants with no option structure, global SKU uniqueness). This slice fixes them at the schema level rather than carrying them forward.

---

## 1 · Scope

**In scope (slice 1):**

- New `services/marketplace-api/` Go binary — first of its kind in the rewritten codebase
- Products module: schema, repositories, services, admin + storefront HTTP, DTOs
- Categories module: tree CRUD, used only from product forms (no dedicated `/categories` page)
- Media module: signed-URL GCS upload, refcount-aware deletion
- Variants from day one: up to 3 options, N-dimensional matrix, per-variant SKU/price/stock
- OpenFGA authorization model + middleware for every admin route
- Admin UI: flat `Products` sidebar item, list page, single-column detail page with inline variant matrix, inline category picker, media upload, "Copy to store" dialog
- Public storefront read endpoints (products list/detail, categories) with a strictly separate DTO family
- Full test suite: unit, repository-integration, service-integration (real Postgres + FGA + GCS emulator), API-integration, three Playwright E2E journeys

**Deferred (slice 2+):** product import/export, bulk duplicate within store, inventory history, multi-warehouse stock, collections, gift cards, subscriptions, bundles, digital products, reviews, Q&A, variant drag-reorder in admin, crop-on-upload, automatic currency conversion in copy-to-store, full-text search tuning beyond GIN baseline, `permissions-matrix` admin UI, Istio peer auth wiring for the trusted `X-Store-ID` header.

---

## 2 · Key decisions (locked)

1. **Single Products surface, no Categories or Inventory sidebar items.** Categories are managed inline from the product form via a searchable combobox with `+ Create new`. Inventory lives on variants, not as a separate page. Rationale: most of the time merchants create categories at the moment they create a product; separate pages become graveyards. Matches brand principle of restraint.
2. **Variants from day one.** Product has ≤3 option axes (Size, Color, Material). Every product has ≥1 variant — "simple products" are modeled as one variant with zero options. No split code path. Rationale: Shopify migrators bounce immediately if they can't model their catalog; schema rework later is painful.
3. **Option A — products belong to a single store (`store_id NOT NULL`).** Each store has its own independent catalog. Stores can differ in currency, so a shared catalog would still need per-store price overlays. Simpler schema, cleaner authz, cleaner storefront queries. Multi-store merchants get a "Copy to store" action instead of shared catalogs.
4. **"Copy to store" action for multi-store tenants.** Copies title, description, handle, tags, SEO, options, variants, media (by reference), categories (created if missing, by slug match). Prices stay in source currency — no silent FX conversion. Inventory starts at 0. Status resets to Draft. Single-store tenants never see the action. This is the escape hatch that makes Option A feel powerful without the complexity of a master-catalog model.
5. **Money as `NUMERIC(12,2)`.** Readable in SQL, supports any currency Mark8ly will plausibly handle, clean Go round-trip via `decimal.Decimal`. No cents-as-bigint, no strings.
6. **Separate DTO families for admin and storefront.** `AdminProductResponse` and `StorefrontProductResponse` are distinct Go structs. The type system prevents `cost_price` / `inventory_quantity` from ever leaking to a public route. Storefront exposes only `in_stock: bool` and optionally `low_stock: bool`, never raw quantity.
7. **Fresh service, not a port.** `services/marketplace-api/` is new code following the `platform-api` structural pattern (per-domain folders with handler/service/repository/models). The legacy `services/products/` code is reference material only.
8. **OpenFGA from day one.** Authorization model committed to source, tuples written inside the same transaction as data, middleware wired on every admin route. Storefront reads bypass FGA (public) and rely on repository-level `status=active AND published_at<=now()` filtering as the safety boundary.
9. **Flat sidebar.** `Products` appears directly in the admin sidebar, not nested under a `Catalog` group. A single-item group would be editorial overhead for a hypothetical future second child.

---

## 3 · Backend architecture

### 3.1 · Service layout

```
services/marketplace-api/
├── cmd/
│   ├── marketplace-api/main.go        # Gin server, graceful shutdown
│   ├── migrate/main.go                # golang-migrate runner
│   └── authz-bootstrap/main.go        # Writes FGA model to marketplace store (idempotent)
├── internal/
│   ├── product/
│   │   ├── models.go                  # GORM structs for products/variants/options/etc.
│   │   ├── repository.go              # Tenant+store-scoped queries; ListAdmin vs ListPublished
│   │   ├── service.go                 # Create/Update/Delete/Copy (transactional)
│   │   ├── admin_handler.go           # /admin/... routes
│   │   └── storefront_handler.go      # /storefront/... routes
│   ├── category/
│   │   ├── models.go, repository.go, service.go
│   │   ├── admin_handler.go, storefront_handler.go
│   ├── media/
│   │   ├── models.go, repository.go, service.go
│   │   ├── handler.go
│   │   └── uploader.go                # GCS signed URLs, existence check, tmp→permanent move
│   ├── authz/                         # Permission constants, tuple helpers
│   ├── config/                        # Env-driven config
│   ├── health/                        # /health, /ready
│   └── test/                          # SetupTestDB, SetupTestFGA, SetupTestGCS helpers
├── migrations/
│   ├── 0001_products_initial.up.sql
│   └── 0001_products_initial.down.sql
├── authz/
│   └── model.fga                      # OpenFGA DSL (committed)
├── go.mod
├── Dockerfile                         # Multi-stage, Alpine 3.19
└── README.md
```

### 3.2 · Middleware chain

**Admin routes:** `GIPAuth → TenantMiddleware → StoreMiddleware → fgaMw.Require(<permission>)`

- `GIPAuth` (from `go-shared`) verifies the GIP JWT
- `TenantMiddleware` extracts `tenant_id` into the request context
- `StoreMiddleware` (new, local to `marketplace-api`) validates `:storeId` in the URL path belongs to the caller's tenant; rejects with 404 if not (no existence leak)
- `fgaMw.Require` checks the relevant permission against the marketplace FGA store

**Storefront routes:** `StoreContextMiddleware`

- Trusts an `X-Store-ID` header set by the storefront Next.js server
- Intra-cluster trust only; Istio peer auth enforcement is a follow-up ops task (noted as TODO in slice 1)
- No user authentication

### 3.3 · Cross-module boundaries

`product.Service` imports `category.Service` and `media.Service` via interfaces. Cross-module calls are in-process method calls, never HTTP. Single transaction boundary for any operation that touches multiple modules (e.g., creating a product with variants + media + categories + FGA tuple).

---

## 4 · Database schema

Single migration: `0001_products_initial.{up,down}.sql`. Seven tables. Postgres 15, `gen_random_uuid()` (built-in, no extension).

```sql
BEGIN;

-- categories — per-store tree
CREATE TABLE categories (
    id           uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid         NOT NULL,
    store_id     uuid         NOT NULL,
    parent_id    uuid         REFERENCES categories(id) ON DELETE RESTRICT,
    name         varchar(200) NOT NULL,
    slug         varchar(200) NOT NULL,
    description  text,
    image_url    text,
    position     integer      NOT NULL DEFAULT 0,
    is_active    boolean      NOT NULL DEFAULT true,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    deleted_at   timestamptz,

    CONSTRAINT categories_slug_per_store_unique UNIQUE (store_id, slug),
    CONSTRAINT categories_name_not_blank        CHECK (length(trim(name)) > 0),
    CONSTRAINT categories_slug_format           CHECK (slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$')
);
CREATE INDEX categories_tenant_idx ON categories (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX categories_store_idx  ON categories (store_id)  WHERE deleted_at IS NULL;
CREATE INDEX categories_parent_idx ON categories (parent_id) WHERE deleted_at IS NULL;

-- products — catalog record; no money, no stock
CREATE TABLE products (
    id                    uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid         NOT NULL,
    store_id              uuid         NOT NULL,
    handle                varchar(200) NOT NULL,
    title                 varchar(300) NOT NULL,
    description           text,
    status                varchar(20)  NOT NULL DEFAULT 'draft',
    vendor_id             uuid,
    tags                  text[]       NOT NULL DEFAULT '{}',
    seo_title             varchar(300),
    seo_description       varchar(500),
    primary_category_id   uuid         REFERENCES categories(id) ON DELETE SET NULL,
    published_at          timestamptz,
    created_by            uuid,
    updated_by            uuid,
    created_at            timestamptz  NOT NULL DEFAULT now(),
    updated_at            timestamptz  NOT NULL DEFAULT now(),
    deleted_at            timestamptz,

    CONSTRAINT products_handle_per_store_unique UNIQUE (store_id, handle),
    CONSTRAINT products_status_valid            CHECK (status IN ('draft','active','archived')),
    CONSTRAINT products_title_not_blank         CHECK (length(trim(title)) > 0),
    CONSTRAINT products_handle_format           CHECK (handle ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    CONSTRAINT products_published_requires_active CHECK (
        (status = 'active' AND published_at IS NOT NULL) OR (status <> 'active')
    )
);
CREATE INDEX products_tenant_idx      ON products (tenant_id)               WHERE deleted_at IS NULL;
CREATE INDEX products_store_idx       ON products (store_id)                WHERE deleted_at IS NULL;
CREATE INDEX products_status_idx      ON products (store_id, status)        WHERE deleted_at IS NULL;
CREATE INDEX products_primary_cat_idx ON products (primary_category_id)    WHERE deleted_at IS NULL;
CREATE INDEX products_published_idx   ON products (store_id, published_at DESC)
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX products_search_idx ON products
    USING gin (to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(description,'')))
    WHERE deleted_at IS NULL;
CREATE INDEX products_tags_idx ON products USING gin (tags) WHERE deleted_at IS NULL;

-- product_options — option axes (max 3 per product, enforced in app layer)
CREATE TABLE product_options (
    id         uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id uuid         NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name       varchar(100) NOT NULL,
    position   integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT product_options_name_per_product_unique UNIQUE (product_id, name),
    CONSTRAINT product_options_name_not_blank          CHECK (length(trim(name)) > 0)
);
CREATE INDEX product_options_product_idx ON product_options (product_id);

-- product_option_values — values per axis
CREATE TABLE product_option_values (
    id         uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    option_id  uuid         NOT NULL REFERENCES product_options(id) ON DELETE CASCADE,
    value      varchar(200) NOT NULL,
    position   integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT option_values_value_per_option_unique UNIQUE (option_id, value),
    CONSTRAINT option_values_value_not_blank         CHECK (length(trim(value)) > 0)
);
CREATE INDEX option_values_option_idx ON product_option_values (option_id);

-- product_variants — where money and stock actually live
CREATE TABLE product_variants (
    id                    uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id            uuid            NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    store_id              uuid            NOT NULL,   -- denormalised for (store_id, sku) uniqueness
    sku                   varchar(100)    NOT NULL,
    barcode               varchar(100),
    price                 numeric(12, 2)  NOT NULL,
    compare_at_price      numeric(12, 2),
    cost_price            numeric(12, 2),
    currency_code         char(3)         NOT NULL,   -- denormalised from store
    weight_grams          integer,
    inventory_quantity    integer         NOT NULL DEFAULT 0,
    inventory_policy      varchar(20)     NOT NULL DEFAULT 'deny',
    low_stock_threshold   integer,
    position              integer         NOT NULL DEFAULT 0,
    created_at            timestamptz     NOT NULL DEFAULT now(),
    updated_at            timestamptz     NOT NULL DEFAULT now(),
    deleted_at            timestamptz,

    CONSTRAINT variants_sku_per_store_unique        UNIQUE (store_id, sku),
    CONSTRAINT variants_price_non_negative          CHECK (price >= 0),
    CONSTRAINT variants_compare_price_non_negative  CHECK (compare_at_price IS NULL OR compare_at_price >= 0),
    CONSTRAINT variants_cost_price_non_negative     CHECK (cost_price IS NULL OR cost_price >= 0),
    CONSTRAINT variants_inventory_non_negative      CHECK (inventory_quantity >= 0),
    CONSTRAINT variants_inventory_policy_valid      CHECK (inventory_policy IN ('deny','continue')),
    CONSTRAINT variants_currency_format             CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT variants_compare_gte_price           CHECK (compare_at_price IS NULL OR compare_at_price >= price)
);
CREATE INDEX variants_product_idx   ON product_variants (product_id)        WHERE deleted_at IS NULL;
CREATE INDEX variants_store_sku_idx ON product_variants (store_id, sku)     WHERE deleted_at IS NULL;
CREATE INDEX variants_low_stock_idx ON product_variants (store_id, product_id)
    WHERE deleted_at IS NULL AND low_stock_threshold IS NOT NULL
      AND inventory_quantity <= low_stock_threshold;

-- variant_option_values — join
CREATE TABLE variant_option_values (
    variant_id      uuid NOT NULL REFERENCES product_variants(id)      ON DELETE CASCADE,
    option_value_id uuid NOT NULL REFERENCES product_option_values(id) ON DELETE RESTRICT,
    PRIMARY KEY (variant_id, option_value_id)
);
CREATE INDEX variant_option_values_value_idx ON variant_option_values (option_value_id);

-- product_media — first-class media; variant_id nullable = product-level
CREATE TABLE product_media (
    id          uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  uuid         NOT NULL REFERENCES products(id)        ON DELETE CASCADE,
    variant_id  uuid                  REFERENCES product_variants(id) ON DELETE SET NULL,
    url         text         NOT NULL,
    storage_key text         NOT NULL,   -- GCS object key; refcount target
    alt         varchar(300),
    position    integer      NOT NULL DEFAULT 0,
    media_type  varchar(20)  NOT NULL DEFAULT 'image',
    width       integer,
    height      integer,
    bytes       bigint,
    created_at  timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT product_media_type_valid CHECK (media_type IN ('image','video'))
);
CREATE INDEX product_media_product_idx ON product_media (product_id, position);
CREATE INDEX product_media_variant_idx ON product_media (variant_id) WHERE variant_id IS NOT NULL;
CREATE INDEX product_media_storage_idx ON product_media (storage_key);

-- product_categories — M:N
CREATE TABLE product_categories (
    product_id  uuid NOT NULL REFERENCES products(id)   ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    PRIMARY KEY (product_id, category_id)
);
CREATE INDEX product_categories_category_idx ON product_categories (category_id);

-- Shared updated_at trigger helper
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER categories_set_updated_at BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER products_set_updated_at   BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER variants_set_updated_at   BEFORE UPDATE ON product_variants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;
```

### 4.1 · Schema design notes

- **Partial indexes on `deleted_at IS NULL`.** Every hot query filters soft-deleted rows; partial indexes stay small.
- **`currency_code` and `store_id` denormalised onto `product_variants`.** Removes a join on every checkout/line-item query and makes `(store_id, sku)` uniqueness a single-column constraint. Service layer enforces that `variant.currency_code = store.currency_code` on every write.
- **`CHECK (compare_at_price >= price)`.** The strikethrough "was $X" must be above the current price, caught at the DB boundary.
- **`products_published_requires_active`.** Active products must have `published_at`. Simplifies storefront queries.
- **`ON DELETE RESTRICT` on category FKs.** Loud errors, not silent orphans. Handler translates to user-friendly error envelopes with counts.
- **`ON DELETE CASCADE` on variants/options/option_values/media-by-product.** No meaning without the parent.
- **`product_media.variant_id ON DELETE SET NULL`.** Deleting a variant demotes its media to product-level; prevents accidental image loss.
- **`storage_key` + index for refcount.** Supports "Copy to store" sharing GCS objects safely. GCS object is physically deleted only when the last row referencing it is gone.
- **`tags text[]` + GIN index.** Postgres-native, no join table.
- **No stored `inventory_status`.** Derived in the DTO mapper.
- **No `version` column for optimistic locking.** Add later if needed; not in slice 1.
- **No seed data.** Test fixtures live in `testdata/` and are loaded by tests.

---

## 5 · OpenFGA authorization

### 5.1 · Model (committed to `authz/model.fga`)

```
model
  schema 1.1

type user

type tenant
  relations
    define member: [user]
    define owner:  [user]
    define admin:  [user] or owner
    define staff:  [user] or admin

type category
  relations
    define tenant: [tenant]
    define viewer: member from tenant
    define editor: admin  from tenant

type product
  relations
    define tenant:    [tenant]
    define viewer:    staff  from tenant
    define editor:    admin  from tenant
    define publisher: admin  from tenant
    define deleter:   owner  from tenant
```

### 5.2 · Key decisions

- **Tenant-scoped, not per-object ACLs.** One tuple per product: `product:<id>#tenant@tenant:<tenant_id>`. All other relations resolve transitively through tenant membership. Scalable to large catalogs without tuple bloat.
- **Create operations check tenant-level permissions** (no object ID yet). `MarketplaceCanCreateProducts` / `MarketplaceCanCreateCategories` constants in `internal/authz/`.
- **Tuple writes happen inside the same GORM transaction as the data writes.** If FGA write fails, the data rolls back. Invariant: permissions never drift from data.
- **Storefront reads bypass FGA entirely.** Public routes rely on repository-level `status=active AND published_at<=now()` filtering plus the distinct storefront DTO type. FGA checks on anonymous traffic would be pointless latency.
- **Staff see the list read-only.** Admins have full CRUD. Owners can hard-delete. Frontend gates are cosmetic; backend is the source of truth.

### 5.3 · Permission map

| Route | Permission |
|---|---|
| `GET /admin/.../products[/:id]` | `product#viewer` |
| `POST /admin/.../products` | `MarketplaceCanCreateProducts` (tenant-level) |
| `PATCH /admin/.../products/:id` | `product#editor` |
| `PATCH /admin/.../products/:id` with `status: active` | `product#publisher` |
| `DELETE /admin/.../products/:id` | `product#deleter` |
| `POST /admin/.../products/:id/copy` | `product#editor` on source + `MarketplaceCanCreateProducts` on target |
| `POST /admin/.../products/:id/media*` | `product#editor` |
| `GET /admin/.../categories` | `category#viewer` |
| `POST /admin/.../categories` | `MarketplaceCanCreateCategories` (tenant-level) |
| `PATCH/DELETE /admin/.../categories/:id` | `category#editor` |

---

## 6 · HTTP API surface

### 6.1 · Admin routes

All nested under `/api/v1/admin/stores/:storeId/` so the store scope is explicit in the URL path and in traces.

```
# Products
GET    /api/v1/admin/stores/:storeId/products
POST   /api/v1/admin/stores/:storeId/products
GET    /api/v1/admin/stores/:storeId/products/:id
PATCH  /api/v1/admin/stores/:storeId/products/:id
DELETE /api/v1/admin/stores/:storeId/products/:id
POST   /api/v1/admin/stores/:storeId/products/:id/copy          # body: { target_store_id }

# Variants (quick updates only — full variant-set changes go through product PATCH)
PATCH  /api/v1/admin/stores/:storeId/products/:id/variants/:variantId

# Media
POST   /api/v1/admin/stores/:storeId/products/:id/media/upload-url
POST   /api/v1/admin/stores/:storeId/products/:id/media
PATCH  /api/v1/admin/stores/:storeId/products/:id/media/:mediaId
DELETE /api/v1/admin/stores/:storeId/products/:id/media/:mediaId

# Categories
GET    /api/v1/admin/stores/:storeId/categories
POST   /api/v1/admin/stores/:storeId/categories
PATCH  /api/v1/admin/stores/:storeId/categories/:id
DELETE /api/v1/admin/stores/:storeId/categories/:id
```

**Why `:storeId` in the path, not a header:** explicit in logs/traces, impossible for a frontend bug to silently mutate the wrong store, and matches the `platform-api` store route pattern.

**Why nested variant PATCH but no nested variant POST/DELETE:** adding or removing a variant changes the product's option matrix, which is a product-level mutation. Quick updates (price, stock, SKU) on an existing variant row are safe and get their own endpoint.

### 6.2 · Storefront routes

```
GET /api/v1/storefront/products
GET /api/v1/storefront/products/:handle              # by handle, not id
GET /api/v1/storefront/categories
GET /api/v1/storefront/categories/:slug/products
```

- `Cache-Control: public, s-maxage=60, stale-while-revalidate=300`
- Weak `ETag` based on the store's updated-products watermark
- `StoreContextMiddleware` extracts `X-Store-ID` from the trusted intra-cluster header
- The **only** repository method these handlers can call is `ListPublished` / `GetPublishedByHandle`, which hard-codes `status='active' AND published_at<=now() AND deleted_at IS NULL`. There is no way to accidentally return a draft.

### 6.3 · DTO families

Two distinct Go struct families — the type system prevents field leakage at compile time.

```go
type AdminProductResponse struct {
    ID, StoreID, Handle, Title string
    Description                *string
    Status                     string   // draft|active|archived
    Tags                       []string
    SEOTitle, SEODescription   *string
    PrimaryCategoryID          *string
    Categories                 []AdminCategoryRef
    Options                    []AdminProductOption
    Variants                   []AdminVariantResponse
    Media                      []AdminMediaResponse
    PublishedAt                *time.Time
    CreatedAt, UpdatedAt       time.Time
}

type AdminVariantResponse struct {
    ID, SKU           string
    Barcode           *string
    Price             decimal.Decimal
    CompareAtPrice    *decimal.Decimal
    CostPrice         *decimal.Decimal   // admin only
    CurrencyCode      string
    WeightGrams       *int
    InventoryQuantity int                // admin only
    InventoryPolicy   string
    LowStockThreshold *int
    OptionValues      []VariantOptionRef
    Position          int
}

type StorefrontProductResponse struct {
    ID, Handle, Title          string
    Description                *string
    Tags                       []string
    SEOTitle, SEODescription   *string
    Categories                 []StorefrontCategoryRef
    Options                    []StorefrontProductOption
    Variants                   []StorefrontVariantResponse
    Media                      []StorefrontMediaResponse
    PriceRange                 StorefrontPriceRange   // min/max across variants
    PublishedAt                time.Time
}

type StorefrontVariantResponse struct {
    ID             string
    Price          decimal.Decimal
    CompareAtPrice *decimal.Decimal
    CurrencyCode   string
    InStock        bool               // derived from quantity
    LowStock       bool               // derived from threshold
    OptionValues   []VariantOptionRef
    // Note: no InventoryQuantity, no CostPrice, no audit fields.
}
```

### 6.4 · Create-product transaction flow

The most complex service operation. All inside one GORM transaction.

1. **Pre-tx validation:** ≤3 options, variant count = Π(option values), variant `option_values` references match declared options exactly, all `category_ids` belong to `:storeId`. Force `variant.currency_code = store.currency_code` (client cannot set currency).
2. Begin tx
3. Insert `products` row
4. Insert `product_options` rows; build `name → option_id` map
5. Insert `product_option_values` rows; build `(option_name, value) → option_value_id` map
6. Insert `product_variants` rows; resolve option_value refs; insert `variant_option_values`
7. Insert `product_categories` join rows
8. For each media item: verify GCS object exists at `storage_key`, move from `tmp/` to permanent path, insert `product_media` row
9. Write FGA tuple `product:<new_id>#tenant@tenant:<tenant_id>`
10. Commit
11. On failure: rollback + best-effort GCS cleanup of moved objects (orphaned tmp objects expire via 24h GCS lifecycle rule)

### 6.5 · Error envelope

Matches `platform-api`:

```json
{ "error": "<code>", "message": "<human-readable>", "details": { } }
```

Error codes: `validation_failed`, `handle_taken` (with suggested alternative), `sku_taken` (with conflicting SKU), `category_not_empty` (with product count), `category_has_children` (with child count), `forbidden`, `not_found`, `upload_not_found`.

---

## 7 · Admin UX

### 7.1 · Sidebar

```
Dashboard
Products              ← flat
Orders
Customers
Settings
```

No `Catalog` group, no Categories or Inventory items. If Collections or similar lands later, either promote `Products → Catalog` as a group, or keep flat with a sibling — one day's work when it happens.

### 7.2 · Products list page (`/products`)

- **Editorial page header:** small-caps eyebrow (future breadcrumb), serif `Products` title, store switcher top-right (hidden for single-store tenants), `+ New product` primary action
- **Summary line** in muted text: "42 products · 3 drafts · 2 archived"
- **Filter row:** search input, `filter-panel` with Status / Category / Stock, `Clear` moss text link (visible only when filters active)
- **`data-table`** with hairline rules (no bordered table frame), 56px row height, columns: checkbox · image (40×40, Paper placeholder) · product (serif title + muted handle) · status (dot + label) · stock (muted for drafts, vermillion for low) · price (`from €89` for variants) · overflow menu
- **Bulk actions bar** (`bulk-actions-bar`) slides up on selection: Set active, Set draft, Archive, Delete, Copy to store (conditional)
- **Empty states** via `empty-state`: zero products (editorial copy + single CTA), no matches (`Clear filters` link)
- **Error state** via `error-boundary` + `error-state`
- **Loading** via `table-skeleton`
- **Pagination** via `pagination` (numbered, not infinite scroll — shareable URLs, keyboard friendly)

### 7.3 · Product detail page (`/products/new` and `/products/:id`)

Single scrolling column. No tabs.

**Layout sections, top to bottom:**

1. **Header strip** — back link, breadcrumb (`Products`), serif title + muted handle, status/visibility compact strip on the right (status `select`, published date, "Copy to another store…" ghost button for multi-store)
2. **Title · Handle · Description** — `input`, `input` with live `mark8ly.com/<handle>` hint, `rich-text-editor`
3. **Media** — tiles grid + `file-upload`, drag to reorder, first image is cover, alt-text popover per tile, variant-attach popover
4. **Categories** — chips (`tag`) with remove buttons + `+ Add category` combobox with inline `+ Create "<text>"`; primary category `select` below
5. **Options & variants** — radio: "simple product" | "has variants"
   - Simple: jump straight to Pricing / Inventory sections
   - Variants: up to 3 option axes (name + `tag` values); generated variant matrix as `data-table` with `inline-editable` cells for SKU, price, stock; "Bulk edit all" popover
6. **Pricing / Inventory** — only for simple products; hidden when variants
7. **Search engine (SEO)** — title and description with fallback hints
8. **Sticky action bar** — moss `Save`, ghost `Discard`; unsaved-changes `confirm-dialog` guard

**Design choices:**

- **Single column, no right rail.** Preserves focus; editorial over utilitarian.
- **Status as a subtle `select`, not a giant "Publish" button.** Saving with `status=active` publishes. Convention over ceremony.
- **Variants inline, not in a modal.** Keeps context; matrix is visible alongside the option definitions that generate it.
- **Matrix preservation on option change:** variants are matched by `(option_value_id tuple)` across regenerations, so SKU/price/stock are preserved when adding or reordering values. Removing a value prompts confirmation with the count of affected variants.
- **Price/stock for variant products lives only in the matrix.** One place, no ambiguity.
- **Autosave deliberately not used.** Explicit Save + Draft status is calmer and safer.

### 7.4 · Inline category picker

- `combobox` with searchable tree (indented children)
- Inline `+ Create "<typed text>"` when no exact match
- New category lands at root; reorganisation is a separate drawer action
- Tree fetched once per page load (`GET /admin/.../categories`), cached client-side

### 7.5 · Category drawer

- `sheet` slide-in from the inline picker's footer (`Manage all categories…`)
- `tree` component with drag-to-reparent, inline rename, delete
- Delete refusal surfaces the error envelope's count ("Remove or reassign 12 products before deleting this category.")
- Deliberately out of the critical path — first-time merchants never open it

### 7.6 · Media upload flow

1. Frontend: `POST /media/upload-url` → receives `{upload_url, storage_key}`
2. Frontend: PUT file directly to GCS `upload_url` with inline progress bar on the tile
3. Frontend: `POST /media` with `{storage_key, alt, position}` → backend verifies GCS, inserts row, returns
4. For `/products/new` (no product ID yet): upload to `tmp/` prefix immediately; on Save, backend moves objects to permanent path inside the transaction. Orphan `tmp/` objects expire via 24h GCS lifecycle rule.

**Reorder** via drag; `PATCH /media/:id` with new positions, batched.

**Variant-specific media:** tile popover `Attach to variant ▾`; storefront uses variant-specific media when the shopper picks that variant, falls back to product-level.

**Alt text:** nudge, not block. Save success toast reports count of missing alts.

### 7.7 · Copy-to-store dialog

- `dialog` with editorial serif title
- Target store `select`
- "What will be copied" / "What will not" quiet prose (not warning boxes)
  - Copies: title, description, handle, tags, SEO, options, variants, media, categories (created if missing)
  - Does not: convert currency (prices stay in source), copy inventory (starts at 0), publish (lands as Draft)
- Success toast with `Open` moss text link, navigates to new draft with store switcher pre-selected

### 7.8 · Role-based UX gates

- **Staff (viewer):** list + read-only detail. All mutating actions hidden. "Read-only" muted indicator on detail header.
- **Admin (editor/publisher):** full CRUD. Copy-to-store enabled where FGA permits on target.
- **Owner (deleter):** additionally sees `Delete` in overflow; admins see only `Archive`.

Frontend gating is cosmetic. FGA enforces on the backend.

### 7.9 · Accessibility

- Skip link first focusable element (already in `AdminShell`)
- Semantic `<main>`, `<nav>`, `<form>`
- Full keyboard nav; drag handles have keyboard fallbacks (`Space` to pick up, arrows to move, `Space` to drop)
- `2px` moss focus ring on all interactive elements
- `prefers-reduced-motion` honored for all transitions
- Inline form errors with `aria-describedby`
- `aria-live="polite"` region for save/copy/delete announcements

### 7.10 · Component reuse map (`@tesserix/web` + `@repo/ui`)

**From `@tesserix/web`:**
`page-header`, `search-bar`, `filter-panel`, `data-table`, `table-skeleton`, `empty-state`, `error-boundary`, `error-state`, `bulk-actions-bar`, `dropdown-menu`, `pagination`, `toast`, `form`, `input`, `textarea`, `label`, `select`, `rich-text-editor`, `file-upload`, `aspect-ratio`, `combobox`, `tag`, `number-input`, `inline-editable`, `popover`, `separator`, `heading`, `skeleton`, `dialog`, `confirm-dialog`, `sheet`/`drawer`, `tree`, `button`, `tenant-switcher`.

**To promote to `@repo/ui` (first use in this feature):**

1. **`StatusDot`** — 8px dot + label; moss/outline/muted variants. Used in products, orders, invites. ~20 LOC.
2. **`PriceDisplay`** — formats `{amount, currencyCode}` in Source Serif 4 tabular figures, locale-aware. Used on admin list, storefront cards, order lines, invoices. ~30 LOC.
3. **`MoneyInput`** — thin wrapper over `@tesserix/web` `number-input` bound to store currency from context, parses to `decimal.js`. Used in product form, later in refunds/coupons. ~40 LOC.

**Admin-only compositions (stay in `apps/admin/components/products/`):**
`ProductsList`, `ProductForm`, `VariantMatrixEditor`, `OptionsEditor`, `MediaGrid`, `CategoryPicker`, `CategoryDrawer`, `CopyToStoreDialog`.

---

## 8 · Implementation milestones

Seven ordered milestones. Each ends with something demonstrable on a running system. Strictly serial except where noted.

### M1 · Scaffold `marketplace-api` service
New service runs locally, answers `/health`, connects to new `marketplace_db`, auth + tenant middleware wired, nothing else. Dockerfile matches existing service Dockerfiles. Added to local dev runner.

**Exit:** `curl /health` returns 200 from the new service; `marketplace_db` exists and is empty.

### M2 · Schema migration + domain models
All seven tables exist with full constraints. GORM models compile. One round-trip integration test passing against real Postgres (insert full graph, read back, assert equality). `down` migration cycles cleanly.

**Exit:** `up → down → up` works; one integration test green.

### M3 · Product + category repositories and services (admin path)
Full service-layer logic for Create/Update/Delete/Copy + category tree ops + variant matrix diff + media refcount. All tested against real Postgres via service integration tests. No HTTP handlers yet.

**Exit:** `go test ./internal/product/... ./internal/category/...` passes; a script can drive a full product lifecycle via service calls.

### M4 · OpenFGA model + authz middleware
Model committed as `authz/model.fga`. Bootstrap program writes it to the marketplace store idempotently. Tuple writes wired into `service.Create` / `service.Delete` inside the same transaction. Real FGA test container in CI. Integration tests cover tuple creation, deletion, cross-tenant denial.

**Exit:** authz integration tests pass against real FGA; tuples and data cannot drift.

### M5 · HTTP handlers + DTOs + admin API routes
Full admin HTTP surface from §6.1 live, typed, validated, authz-gated. `StoreMiddleware` enforces store-belongs-to-tenant. Error envelope matches `platform-api`. API-level integration tests cover full lifecycle, unauthorized paths, validation failures, collision handling.

**Exit:** curl-able admin API complete; CI runs API integration tests against real Postgres + FGA + GCS emulator.

### M6 · Storefront read routes
Four storefront endpoints live. Distinct DTO family enforced at the type level. `ListPublished` is the only repository method reachable from storefront handlers. Cache headers set. Leak tests assert draft/archived/unpublished products never return, and that `cost_price`/`inventory_quantity` never appear in storefront JSON.

**Exit:** storefront endpoints pass leak tests; cache headers correct.

**M6 can run in parallel with M7a if needed.**

### M7 · Admin UI
Four sub-milestones, strictly serial. Each is its own commit.

- **M7a — Sidebar + list page.** `Products` sidebar item, list page with all the components from §7.10, server-side data fetch, `StatusDot` and `PriceDisplay` promoted to `@repo/ui`.
- **M7b — Detail page (no variants yet).** Title, description, status, categories with inline picker, SEO, sticky action bar, unsaved guard. Create and simple-update flows working.
- **M7c — Media + variants.** `MoneyInput` promoted. `MediaGrid` with tmp-upload flow. `OptionsEditor`. `VariantMatrixEditor` with matrix preservation. Simple-vs-variants toggle. Full variant-product create working end-to-end.
- **M7d — Copy-to-store + category drawer + polish.** `CopyToStoreDialog`. Bulk copy from list. `CategoryDrawer` with tree edit. Keyboard/motion/focus audit. Toast + `aria-live` wiring. Three Playwright E2E journeys from §9.5 passing.

**Exit:** a merchant can onboard ~10 products (including variant products and photos) end-to-end with no workarounds; E2E journeys green in CI.

### Milestone dependency graph

```
M1 → M2 → M3 → M4 → M5 → M6
                    └───→ M7a → M7b → M7c → M7d
```

---

## 9 · Testing strategy

**Rule:** real dependencies at every boundary that matters. No mocked Postgres, no mocked FGA, GCS via `fake-gcs-server` emulator (protocol-level substitute, not a mock).

### 9.1 · Unit tests (pure logic, no I/O)

- Variant matrix generation (0/1/2/3 options, edge cases)
- Variant matrix diff (preserve by `option_value_id` tuple match)
- DTO mappers (assert `StorefrontProductResponse` cannot contain `cost_price`/`inventory_quantity`)
- `in_stock` / `low_stock` / `price_range` derivation
- Handle/slug generators (unicode, length, collision)
- Request validators (≤3 options, matrix integrity, currency enforcement)

**Target:** 90%+ on pure logic. Runs in milliseconds.

### 9.2 · Repository integration tests (real Postgres)

- Full product-graph round-trip
- Tenant scoping (A cannot see B's products — **most critical class**)
- Store scoping, `(store_id, handle)` and `(store_id, sku)` uniqueness
- Soft delete isolation
- `ListPublished` safety (draft/archived/unpublished excluded)
- Cascade behaviour (product delete → options/variants/media gone; variant delete → media demoted)
- Unique-constraint violations return typed errors
- Concurrent-insert race (two goroutines, one wins cleanly)
- Transaction rollback on mid-flight failure
- Full-text search, tag array filter, category tree operations

Runs in CI every commit, under a minute with `t.Parallel()`.

### 9.3 · Service integration tests (real Postgres + real FGA + GCS emulator)

- Full `Create` inside one transaction, asserting DB + FGA + GCS consistency
- Mid-flight failure rolls back everything including GCS tmp→permanent moves
- **FGA tuple write failure rolls back the DB** (permissions-data invariant)
- `Copy` end-to-end across stores: media by reference, categories created if missing, inventory at 0, draft status, source currency preserved
- Refcount-aware media deletion (shared `storage_key` across copies)
- Currency enforcement (service overrides client-supplied currency silently)
- Every typed error code from §6.5 has a test producing it
- Cross-tenant access returns `not_found` (no existence leak)
- Category delete refusals with correct counts

### 9.4 · API integration tests (full HTTP stack)

- Full admin lifecycle via HTTP
- Unauthorized paths (no token 401, wrong tenant 403, staff on POST 403)
- Store scoping via URL (other tenant's store → 404)
- Validation error envelope shape
- Handle and SKU collision responses
- **Storefront leak tests**: draft→404, `cost_price` absent from response JSON (via anonymous-struct unmarshal check)
- Cache header correctness
- Media upload dance end-to-end against GCS emulator
- Concurrent create: 10 simultaneous requests, exactly 1 wins

### 9.5 · Playwright E2E (three critical journeys)

**Journey 1 — Simple product end-to-end**
Admin creates a simple product with image, category, price, stock; flips to Active; verifies it renders on the storefront URL; asserts `cost_price` absent from storefront HTML/JSON.

**Journey 2 — Variant product with matrix editing**
Create "Porcelain bowl" with Size (S/M/L) + Color (Natural/Ink); inline-edit SKUs; bulk-edit price to €42; set one variant to low stock; verify list shows "3 left" in vermillion; remove option value "Large" with confirmation; verify 4 variants remain with S/M data preserved.

**Journey 3 — Copy to store across currencies**
Tenant has EUR and USD stores. Copy "Porcelain bowl" from EUR to USD. Verify: title/media/handle copied, variants copied, prices still in EUR, inventory all 0, status Draft, source unchanged.

### 9.6 · Deliberately not tested

- Unit-testing GORM query builders against mock DBs
- E2E duplication of every API path
- Mocked FGA or DB anywhere
- Load/perf tests in slice 1
- Visual regression (handled by design system)
- Fuzz testing the option matrix

### 9.7 · CI wiring

- Postgres 15, FGA container, `fake-gcs-server` in CI matrix
- `go test -race -coverprofile=coverage.out ./...`
- `golangci-lint run` (matches `platform-api` config)
- 80% coverage gate on business logic (`internal/product/`, `internal/category/`, `internal/media/`)
- Playwright E2E added to `apps/admin/` CI workflow after marketplace-api + admin both build and boot in ephemeral env
- Runtime budget: marketplace-api CI <5 min, admin E2E <3 min

---

## 10 · Risks and mitigations

| Risk | Mitigation |
|---|---|
| FGA + DB drift if a tuple write fails silently | Tuple writes inside the same GORM transaction as data inserts; failure rolls back DB. Covered by service-integration tests. |
| Storefront leaks draft products | Repository-level `ListPublished` is the only method reachable from storefront handlers; distinct DTO types prevent field leakage; leak tests at both repository and API layer. |
| Variant matrix regeneration loses SKU/price/stock | Matrix diff matches by `option_value_id` tuple; unit-tested with 15+ cases; removing a value prompts user confirmation. |
| GCS orphan objects from failed uploads | 24h lifecycle rule on `tmp/` prefix; nightly sweep for permanent-path objects with no `product_media` row (future). |
| Multi-store currency confusion on Copy | Copy dialog explicitly states prices are NOT converted; copied product lands as Draft so merchants must review before going live. |
| Merchants expect shared catalogs across stores | Copy-to-store is the escape hatch for slice 1; real demand for shared catalogs → future Option D migration (add nullable `master_product_id`, forward-compatible). |
| Tenant leakage via missed repository filter | `TenantMiddleware` + repository methods that always take context; integration tests specifically assert cross-tenant isolation. |
| Over-reliance on `go-shared` patterns that don't exist yet in new monorepo | Slice 1 copies middleware from `platform-api` where `go-shared` is already working; no new shared-lib dependencies introduced. |

---

## 11 · Open questions (to address during implementation, not blockers)

1. **Reusable test helpers.** Does `platform-api` already expose a test setup package we should import, or do we copy its patterns? Defer to M2 when the first test is written.
2. **Exact shape of `StoreMiddleware`.** Does `platform-api` already have one we can lift? If so, promote to `go-shared`. Defer to M1.
3. **`X-Store-ID` trust boundary.** Slice 1 trusts the header intra-cluster with a TODO. Istio peer auth config is a follow-up ops task — not a slice 1 blocker.
4. **FGA model version migration.** First-time bootstrap is straightforward; subsequent model changes need a story. Address when slice 2 needs it.
5. **Tabular figures in Source Serif 4.** Verify the variant feature is available in the loaded weight set; fall back to Source Sans 3 tabular if not. Confirm in M7a.

---

## 12 · Definition of done (slice 1)

- [ ] `marketplace-api` service exists, is in the CI pipeline, deploys via the existing Knative + ArgoCD path
- [ ] `marketplace_db` migration committed, runs in dev and prod
- [ ] OpenFGA marketplace store model committed, bootstrap runs idempotently on deploy
- [ ] Admin CRUD for products, variants, options, media, categories working end-to-end
- [ ] Storefront read endpoints serve active products with correct cache headers and no draft leakage
- [ ] Admin UI: `Products` sidebar, list page, detail page, variant matrix editor, media upload, inline category picker, category drawer, copy-to-store dialog — all in the Paper · Ink · Moss editorial system
- [ ] Role-based gating in place (staff read-only, admin CRUD, owner delete)
- [ ] `StatusDot`, `PriceDisplay`, `MoneyInput` promoted to `@repo/ui`
- [ ] Test suite green: unit, repository-integration, service-integration, API-integration, three Playwright E2E journeys
- [ ] 80%+ coverage on business logic
- [ ] A merchant can onboard ~10 products (simple + variant + photos + categories) end-to-end with no workarounds
- [ ] Documentation in `services/marketplace-api/README.md` covers: local dev setup, test strategy, module boundaries
