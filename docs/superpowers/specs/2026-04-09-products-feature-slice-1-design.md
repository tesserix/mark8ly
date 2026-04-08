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

**Deferred (slice 2+):** product import/export, bulk duplicate within store, inventory history, multi-warehouse stock, collections, gift cards, subscriptions, bundles, digital products, reviews, Q&A, variant drag-reorder in admin, crop-on-upload, automatic currency conversion in copy-to-store, full-text search tuning beyond GIN baseline, `permissions-matrix` admin UI, per-object OpenFGA ACLs (e.g. "this admin can only edit these N products").

---

## 2 · Key decisions (locked)

1. **Single Products surface, no Categories or Inventory sidebar items.** Categories are managed inline from the product form via a searchable combobox with `+ Create new`. Inventory lives on variants, not as a separate page. Rationale: most of the time merchants create categories at the moment they create a product; separate pages become graveyards. Matches brand principle of restraint.
2. **Variants from day one.** Product has ≤3 option axes (Size, Color, Material). Every product has ≥1 variant — "simple products" are modeled as one variant with zero options. No split code path. Rationale: Shopify migrators bounce immediately if they can't model their catalog; schema rework later is painful.
3. **Option A — products belong to a single store (`store_id NOT NULL`).** Each store has its own independent catalog. Stores can differ in currency, so a shared catalog would still need per-store price overlays. Simpler schema, cleaner authz, cleaner storefront queries. Multi-store merchants get a "Copy to store" action instead of shared catalogs.
4. **"Copy to store" action for multi-store tenants.** Copies title, description, handle, tags, SEO, options, variants, media (by reference), categories (created if missing, by slug match). Prices stay in source currency — no silent FX conversion. Inventory starts at 0. Status resets to Draft. Single-store tenants never see the action. This is the escape hatch that makes Option A feel powerful without the complexity of a master-catalog model.
5. **Money as `NUMERIC(12,2)`.** Readable in SQL, supports any currency Mark8ly will plausibly handle, clean Go round-trip via `decimal.Decimal`. No cents-as-bigint, no strings.
6. **Separate DTO families for admin and storefront.** `AdminProductResponse` and `StorefrontProductResponse` are distinct Go structs. The type system prevents `cost_price` / `inventory_quantity` from ever leaking to a public route. Storefront exposes only `in_stock: bool` and optionally `low_stock: bool`, never raw quantity.
7. **Fresh service, not a port.** `services/marketplace-api/` is new code following the `platform-api` structural pattern (per-domain folders with handler/service/repository/models). The legacy `services/products/` code is reference material only.
8. **OpenFGA from day one, tenant-scoped only.** Authorization model committed to source; middleware wired on every admin route. **No per-product or per-category tuples are written.** Every relation in the model resolves `from tenant`, so authorization reduces to "is this user a staff/admin/owner of this tenant?" — which is already represented by the platform's existing `tenant:<id>#member/admin/owner@user:<uid>` tuples. Writing a `product:<id>#tenant@tenant:<tid>` tuple at create time would carry no information beyond `products.tenant_id` itself, while introducing a dual-write drift risk between Postgres and the OpenFGA store (two different services, two different databases, no 2PC). Dropping per-object tuples eliminates that invariant we couldn't actually guarantee. Per-object ACLs are a future extension if and when real "this admin can only edit these N products" requirements surface. Storefront reads bypass FGA (public) and rely on repository-level `status=active AND published_at<=now()` filtering plus the distinct storefront DTO family as the safety boundary.
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

---

## 13 · Revisions after review (v1 → v1.1)

The sections below supersede the corresponding content earlier in this document where they conflict. They consolidate feedback from four independent reviews (spec reviewer, architect, UX specialist, tech lead) plus the user directive **"avoid dialogs and modals unless for delete"**.

When a section here contradicts §§1–12, §13 wins.

### 13.1 · Architecture corrections

#### 13.1.1 · OpenFGA: per-object tuples dropped (supersedes §5.2, §5.3, §6.4 step 9)

Per-object tuples (`product:<id>#tenant@tenant:<tid>`, and the equivalent for categories) are **not written**. Every relation in the model resolves `from tenant`, so the tuple carries no information beyond `products.tenant_id` / `categories.tenant_id`. Writing it creates a dual-write drift risk between Postgres and the OpenFGA store (two services, two databases, no 2PC) without adding any security.

**Consequences:**

- The `category` and `product` FGA types are removed from `model.fga`. The model contains only `user` and `tenant` with `member`/`owner`/`admin`/`staff` relations. Tenant membership tuples are already written by `platform-api` during onboarding/invitation — `marketplace-api` is a pure reader of them.
- All admin route permission checks reduce to tenant-level membership checks: `fgaMw.RequireTenantRelation(tenantRole)` where `tenantRole` is one of `staff`, `admin`, `owner`.
- `service.Create` / `service.Update` / `service.Delete` no longer write or delete FGA tuples. Transactions become DB-only.
- The permission map in §5.3 is replaced by:

| Route | Required tenant role |
|---|---|
| `GET /admin/.../products[/:id]` | `staff` |
| `POST /admin/.../products` | `admin` |
| `PATCH /admin/.../products/:id` | `admin` |
| `PATCH /admin/.../products/:id` with `status: active` | `admin` |
| `DELETE /admin/.../products/:id` (hard delete) | `owner` |
| `POST /admin/.../products/:id/copy` | `admin` (enforced on both source and target store — both stores must belong to the same tenant where the caller is admin) |
| `POST /admin/.../products/:id/media*` | `admin` |
| `GET /admin/.../categories` | `staff` |
| `POST /admin/.../categories` | `admin` |
| `PATCH /admin/.../categories/:id` | `admin` |
| `DELETE /admin/.../categories/:id` | `admin` |

- "Archive" is a status update and uses the `admin` role like any other PATCH. Only hard `DELETE` requires `owner`.
- All admin routes return `404 not_found` (not `403 forbidden`) when the caller's tenant doesn't own the target store — no existence leaks across tenants.

#### 13.1.2 · Storefront trust boundary (supersedes §3.2, §6.2)

No trusted `X-Store-ID` header. The storefront boundary uses three layers of defense:

1. **Separate Gin engine on port 8081** — the marketplace-api binary starts two engines: admin on `:8080` (main port, exposed to admin VirtualService only) and storefront on `:8081` (exposed to storefront VirtualService only). Routes registered on one engine are unreachable from the other. Cheap belt-and-braces isolation, reversible, requires no Istio work.
2. **Path-based store resolution** — storefront routes take the store slug in the URL: `GET /api/v1/storefront/stores/:storeSlug/products`. The handler calls `stores.GetBySlug(slug)` against marketplace-api's own local `stores` projection table (see §13.1.3). No header trust.
3. **Shared-secret header** `X-Storefront-Key` — env-configured, rotatable, rejected if missing. Stops accidental direct access from outside the storefront Next.js server even if the two ports are ever exposed. Removed once Istio peer auth lands (tracked as a follow-up ops task, but the slice 1 code ships without relying on it).

Storefront routes after revision:

```
GET /api/v1/storefront/stores/:storeSlug/products
GET /api/v1/storefront/stores/:storeSlug/products/:handle
GET /api/v1/storefront/stores/:storeSlug/categories
GET /api/v1/storefront/stores/:storeSlug/categories/:slug/products
```

#### 13.1.3 · `stores` projection table in marketplace-api (new; §4 addendum)

`marketplace-api` needs its own read-only copy of store metadata so `StoreMiddleware` does not call `platform-api` on every admin request (pool killer on db-f1-micro with 5 connections per service).

New table in migration `0001`:

```sql
CREATE TABLE stores (
    id                         uuid         PRIMARY KEY,
    tenant_id                  uuid         NOT NULL,
    slug                       varchar(63)  NOT NULL,
    name                       varchar(200) NOT NULL,
    country_code               char(2)      NOT NULL,
    currency_code              char(3)      NOT NULL,
    timezone                   varchar(64)  NOT NULL,
    status                     varchar(20)  NOT NULL,
    products_updated_watermark timestamptz  NOT NULL DEFAULT now(),
    synced_at                  timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT stores_slug_unique UNIQUE (slug)
);
CREATE INDEX stores_tenant_idx ON stores (tenant_id);
```

**Sync strategy for slice 1:** lazy pull-through. On any admin request that names a `:storeId`, `StoreMiddleware` reads the local `stores` row; if missing or `synced_at` is older than 5 minutes, it fetches `/internal/tenants/:tid/stores/:sid` from `platform-api` once, upserts the row, and serves the request. All subsequent requests for the same store inside 5 minutes are local-only. A slice 2 task replaces pull-through with an outbox-driven event when platform-api gains its outbox.

`products_updated_watermark` is bumped on every product/variant/media/category mutation in the same transaction as the mutation itself (one extra `UPDATE stores SET products_updated_watermark = now() WHERE id = $1` inside the tx). The storefront ETag uses this watermark.

#### 13.1.4 · `StoreMiddleware` specification (supersedes §3.2 open question)

```go
func StoreMiddleware(storeRepo stores.Repository, platformClient platform.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        storeID := c.Param("storeId")
        tenantID := auth.TenantID(c)  // set by TenantMiddleware upstream

        store, err := storeRepo.GetByIDForTenant(c, storeID, tenantID)
        if errors.Is(err, stores.ErrNotFound) || stores.IsStale(store, 5*time.Minute) {
            store, err = refreshFromPlatform(c, platformClient, storeRepo, storeID, tenantID)
        }
        if err != nil || store == nil {
            // Not found, wrong tenant, or sync failure — 404, no leak
            apperrors.Respond(c, apperrors.NotFound("store"))
            c.Abort()
            return
        }
        c.Set("store", store)
        c.Next()
    }
}
```

Store ownership is verified at every hop: the local lookup is keyed by `(store_id, tenant_id)`, so a caller's tenant can only ever see their own stores. Cross-tenant `:storeId` values produce 404 with no existence leak.

### 13.2 · Schema corrections (supersedes §4)

The tables from §4 are kept with the following modifications. The final table count is **twelve** (not seven): `categories`, `products`, `product_options`, `product_option_values`, `product_variants`, `variant_option_values`, `product_media`, `product_categories`, `stores` (projection), `variant_stock` (new), `outbox_events` (new), `idempotency_keys` (new).

#### 13.2.1 · Unique constraints → partial unique indexes (blocking fix)

Soft-delete + plain `UNIQUE` means a deleted row blocks reuse of its handle/slug/sku forever. All three uniqueness constraints on soft-deletable tables become partial unique indexes:

```sql
-- Replace these three constraints from §4:
-- CONSTRAINT categories_slug_per_store_unique UNIQUE (store_id, slug)
-- CONSTRAINT products_handle_per_store_unique UNIQUE (store_id, handle)
-- CONSTRAINT variants_sku_per_store_unique    UNIQUE (store_id, sku)
-- With:
CREATE UNIQUE INDEX categories_slug_per_store_live_unique
    ON categories (store_id, slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX products_handle_per_store_live_unique
    ON products (store_id, handle) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX variants_sku_per_store_live_unique
    ON product_variants (store_id, sku) WHERE deleted_at IS NULL;
```

#### 13.2.2 · `variants_inventory_non_negative` CHECK removed (blocking fix)

The CHECK conflicts with `inventory_policy = 'continue'` (oversell allowed). Negative inventory is a legitimate state when a merchant opts into backorder selling. Drop the constraint; enforce non-negative-when-policy-is-deny at the service layer instead, where the policy is known.

#### 13.2.3 · `store_id` consistency FK on variants (blocking fix)

Denormalization without integrity → drift risk. Add a composite unique constraint on `products` and make `product_variants` reference it:

```sql
ALTER TABLE products ADD CONSTRAINT products_id_store_unique UNIQUE (id, store_id);
-- Drop the plain product_id FK on product_variants and replace with composite:
ALTER TABLE product_variants
    DROP CONSTRAINT product_variants_product_id_fkey,
    ADD CONSTRAINT product_variants_product_store_fk
        FOREIGN KEY (product_id, store_id)
        REFERENCES products(id, store_id)
        ON DELETE CASCADE;
```

Now `product_variants.store_id` can never drift from `products.store_id` — the database enforces it.

#### 13.2.4 · `currency_code` drift trigger (important fix)

Store-level currency change (rare, usually a migration) must cascade to all variants or fail loudly. A `BEFORE UPDATE` trigger on `product_variants` rejects any attempt to set a currency_code that doesn't match the current `stores.currency_code`. Store currency changes themselves go through a maintenance path documented in `services/marketplace-api/README.md` (bulk update inside a single tx, locks `product_variants` briefly).

#### 13.2.5 · `products_published_requires_active` logic kept, auto-set documented

The CHECK is correct; the PATCH trap is resolved at the service layer: any `PATCH` that transitions status to `active` sets `published_at = now()` automatically in the same update statement. Transition to `draft` or `archived` does **not** clear `published_at` — leaving it captures "most recent publish time" for audit and future analytics. Documented in the service method; unit-tested.

#### 13.2.6 · `variant_stock` table (new, slice-2 forward-compat)

Multi-warehouse is explicitly deferred, but the schema cements a structure today so slice 2 is additive:

```sql
CREATE TABLE variant_stock (
    variant_id         uuid        NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    location_id        uuid        NOT NULL,                      -- single "default" location in slice 1
    quantity           integer     NOT NULL DEFAULT 0,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (variant_id, location_id)
);
CREATE INDEX variant_stock_location_idx ON variant_stock (location_id);
```

Slice 1 writes exactly one row per variant at the `DEFAULT_LOCATION_ID` (an env var, initially the same UUID for every tenant). `product_variants.inventory_quantity` stays as a denormalised sum maintained in the service layer via `variant_stock` writes (a sum is trivial with one row). Slice 2 adds more locations; reads of `inventory_quantity` keep working; UI gradually migrates to the per-location view.

#### 13.2.7 · `outbox_events` + `idempotency_keys` tables (new)

Cemented write-path infrastructure so slice 2 orders/webhooks are additive:

```sql
CREATE TABLE outbox_events (
    id            uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid         NOT NULL,
    aggregate     varchar(64)  NOT NULL,    -- 'product', 'category', etc.
    aggregate_id  uuid         NOT NULL,
    event_type    varchar(64)  NOT NULL,    -- 'product.created', ...
    payload       jsonb        NOT NULL,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    published_at  timestamptz,
    error         text
);
CREATE INDEX outbox_unpublished_idx ON outbox_events (created_at) WHERE published_at IS NULL;

CREATE TABLE idempotency_keys (
    key          varchar(255) PRIMARY KEY,
    tenant_id    uuid         NOT NULL,
    response     jsonb,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    expires_at   timestamptz  NOT NULL
);
CREATE INDEX idempotency_expires_idx ON idempotency_keys (expires_at);
```

Products module writes `product.created`/`updated`/`deleted` events to `outbox_events` in the same tx as the mutation from day one. The publisher (a background goroutine with a Pub/Sub adapter) is slice 2. `idempotency_keys` is wired into `POST` handlers that accept an `Idempotency-Key` header — slice 1 use is optional but the infrastructure is live.

#### 13.2.8 · `copy_source_product_id` audit column (new)

`products` gets a nullable `copy_source_product_id uuid` column (no FK — the source may be soft-deleted or across stores). Populated during `service.Copy`. Enables future master-catalog backfill and gives merchants a visible "Copied from …" affordance. Not a gate on any behavior in slice 1.

### 13.3 · Create-product transaction flow correction (supersedes §6.4)

GCS operations happen **before** `BEGIN`, not inside the transaction. No network I/O inside an open Postgres connection. Revised flow:

1. **Pre-tx validation** (unchanged): option/variant integrity, category ownership, currency enforcement, variant count cap (≤100 per product — new hard limit), ≤3 options.
2. **Pre-tx GCS phase:** for each media item, verify the object exists at the submitted `storage_key`. Storage keys are **content-addressed** (`tenants/<tid>/products/media/<sha256>/<filename>`), computed by the frontend before upload. No tmp→permanent move; the object is uploaded to its permanent path directly. If any object is missing, return `upload_not_found` before touching the DB.
3. **Begin tx**
4. Insert `products` row
5. Insert `product_options`, `product_option_values`, `product_variants`, `variant_option_values`
6. For each variant, insert one `variant_stock` row at `DEFAULT_LOCATION_ID`
7. Insert `product_categories`
8. Insert `product_media` (refcount is a `count(*)` on `storage_key`; multiple rows sharing a key is expected)
9. Insert `outbox_events` row: `product.created`
10. `UPDATE stores SET products_updated_watermark = now() WHERE id = $storeId`
11. Commit

No FGA tuple write (see §13.1.1). No GCS move step. No mid-flight GCS cleanup path because there is nothing to undo.

**Orphan sweep:** a named slice-1 deliverable. A nightly job (initially a single Go program invoked via CronJob or Cloud Scheduler) lists objects under `tenants/*/products/media/*` older than 24 hours and deletes any with no matching `product_media.storage_key` row. This is the only garbage-collection path; simple, observable, cheap.

### 13.4 · Error codes (supersedes §6.5)

Full enumerated list:

| Code | HTTP | Meaning |
|---|---|---|
| `validation_failed` | 400 | Request shape/content invalid; `details` names the field |
| `variant_matrix_mismatch` | 400 | Variant count doesn't equal Π(option_value_counts); `details` has `expected`/`got` |
| `too_many_options` | 400 | >3 option axes |
| `too_many_variants` | 400 | >100 variants per product |
| `currency_mismatch` | 400 | Variant currency differs from store currency (also silently corrected — this code is only returned when correction is not possible, e.g., copy-to-store target validation) |
| `handle_taken` | 409 | `(store_id, handle)` collision; `details.suggested` has an available alternative |
| `sku_taken` | 409 | `(store_id, sku)` collision; `details.sku` identifies the conflicting value |
| `category_not_empty` | 409 | Delete refused; `details.product_count` |
| `category_has_children` | 409 | Delete refused; `details.child_count` |
| `target_store_invalid` | 400 | Copy-to-store target is missing, same as source, or owned by a different tenant |
| `upload_not_found` | 400 | A submitted `storage_key` doesn't exist in GCS |
| `forbidden` | 403 | Authenticated but lacks the required tenant role |
| `not_found` | 404 | Resource doesn't exist OR belongs to another tenant (no existence leak) |

All tests in §9 that reference "every typed error code from §6.5" use this enumeration.

### 13.5 · UX corrections (supersedes parts of §7)

**No dialogs or modals except for hard delete.** `confirm-dialog` is permitted only for `DELETE` actions (product delete, category delete with cascade). Everything else resolves inline via banners, sticky bars, or dedicated page routes.

#### 13.5.1 · Copy-to-store becomes a dedicated page (supersedes §7.7)

Route: `/products/:id/copy`. Triggered from the product detail overflow menu or the list bulk action. Full editorial single-column page, back button returns to source product. Structure:

```
  ← Back to Linen shirt

  Copy "Linen shirt"
  to another store
  ─────────────────────────────
  Target store
  [ Mark8ly EU ▾ ]

  ⟡  The target store sells in USD.
     Prices on this product are in EUR and will NOT be converted.
     Review every price after copying.

  What carries over
  • Title, description, handle, tags, SEO
  • Options (Size, Color) and every variant
  • Media
  • Categories (created in the target store if missing)

  What does not
  • Inventory — starts at zero in the target store
  • Published status — lands as Draft so you can review
                                       ─────────────────
                               [ Cancel ]   [ Copy ]
```

The currency-mismatch callout (the `⟡` block) is rendered **only when** `source.currency_code != target.currency_code`. It's not a warning box — a single moss `⟡` glyph, a hairline moss left-border, editorial prose, inline with the page. Visually distinct, but restrained. Same-currency copies don't render the callout at all.

On success: navigate directly to the new draft in the target store (store switcher in the page header pre-selected). A brief inline success banner appears at the top of the new product's form for ~4s, with an `Undo` moss text link that soft-deletes the copy.

Bulk copy from the list page uses the same route with query params: `/products/copy?ids=1,2,3` — the page renders the same form keyed to N products instead of 1. No bulk-copy dialog.

#### 13.5.2 · Category management becomes a dedicated page (supersedes §7.5)

Route: `/products/categories/manage`. Linked from the inline picker footer (`Manage all categories →`). Full-page tree editor with drag-reparent, inline rename, delete (delete is the one place `confirm-dialog` shows up per the no-modal rule). Uses `@tesserix/web` `tree` + `input` + editorial layout. Not a drawer, not a sheet. Back button returns to wherever the merchant came from.

#### 13.5.3 · Inline category picker: "Create under [highlighted]" fork (§7.4 addendum)

When a merchant types a new category name and an existing category row is currently highlighted in the combobox, the "+ Create" footer expands to two options side by side:

```
  + Create "linen classics" at root
  + Create "linen classics" under Shirts
```

Only when both options are meaningful (a category is highlighted AND the typed text has no exact match). Prevents the root-category graveyard without adding a modal.

#### 13.5.4 · Variant removal is an inline banner (supersedes the variant confirmation modal assumption in §7.3)

When the merchant removes an option value that has existing variants, an inline banner slides down above the variant matrix:

```
  ⟡  Removing "Large" will remove 2 variants (LIN-L-SAND, LIN-L-INK).
     [ Undo ]   [ Remove variants ]
```

Hairline moss left-border. Inline Undo restores the option value. `Remove variants` soft-deletes them on next Save. No modal. If the merchant discards instead of saving, the soft-delete never happens.

#### 13.5.5 · Unsaved-changes navigation guard (supersedes the `confirm-dialog` unsaved guard in §7.3)

No modal on navigation. Mechanism:

1. The sticky action bar's state changes to an **unsaved-changes prompt**: left-aligned muted text "Unsaved changes", right-aligned `[Discard] [Save]`. Hairline-highlighted top border becomes 2px moss.
2. Navigation attempts (sidebar click, browser back) are intercepted by the Next.js router. Instead of a modal, the action bar subtly pulses (2-step moss-to-default fade, honors `prefers-reduced-motion`) and the page does **not** navigate. A second click within 3s forces navigation and discards changes (power-user shortcut).
3. Tab-close / browser-close fires `beforeunload`, which is the single browser-native prompt users cannot avoid — this is the only "modal-like" exception, and it's the browser's, not ours.

#### 13.5.6 · Handle conflict on already-published product (new, supersedes §7.3 addendum)

On `PATCH` returning `handle_taken`, the handle field renders an inline error beneath it:

```
  Handle
  [ linen-shirt                                     ]
  ⟡  Taken in this store. Try linen-shirt-2 →
```

The `→` is a one-click moss text link that fills the field with the suggested value. No modal. Save button stays enabled so the merchant can retype manually instead.

#### 13.5.7 · Staff read-only mode is explicit (supersedes §7.8)

For staff viewers:

- All form inputs render with `disabled` or `readonly` HTML attributes — not hidden, not ghost-styled. The merchant can see the data structure; they can't type into it.
- The sticky action bar is **not rendered at all** (not hidden with CSS — genuinely absent from the DOM).
- The page header shows a small `staff · read-only` muted label next to the title, linked via `aria-describedby` to a tooltip: *"You can view products but not edit them. Ask your store admin for edit access."*
- Row-level actions in the list page's overflow menu are absent. The row is still clickable (drills into the read-only detail page).

#### 13.5.8 · Variant matrix row grouping + fill-down (supersedes §7.3 — variant editor)

For products with >2 options, or >12 variants, the matrix gets:

1. **Row grouping by the first option axis.** Variants are rendered as collapsible groups keyed to (e.g.) Size. Each group has a compact header showing the axis value + a group-level bulk-edit affordance: `Size: Medium (6 variants) [ Bulk edit → ]`. Collapsed groups show only a single summary row per group. Expanded groups show all child variants.
2. **Column fill-down keyboard shortcut.** Selecting a cell in the price, stock, or SKU column and pressing `⌘D` (or `Ctrl+D`) fills the value down to all visible rows below it in the same group. Announced via `aria-live` on activation.
3. **Column freeze.** The leftmost column (the option-values label) stays pinned when horizontally scrolling; the header row stays pinned when vertically scrolling within the matrix section.
4. **Hard cap: 100 variants.** Products with more than 100 variants are refused by the API (`too_many_variants`); the form surfaces this before save as a live counter.

#### 13.5.9 · Single-column layout: left-margin anchor list for variant-heavy products (supersedes §7.3)

For variant-heavy products (more than 2 options or more than 12 variants), the detail page renders an editorial left-margin **section anchor list** — not a sidebar, not tabs, not a sticky toolbar. A small vertically-centered list of section names (Title · Media · Categories · Variants · SEO), each a moss text link, rendered in the whitespace to the left of the main column at desktop widths ≥1280px. Matches magazine "chapter marks" convention. At narrower widths, the list is absent.

#### 13.5.10 · Progressive disclosure for first-time merchants (new, §7.3 addendum)

1. `Search engine` (SEO) section is **collapsed by default** on new products. Expands on click. Saved products remember the expansion state per session.
2. The `has variants` radio has a one-line descriptor beneath it in muted text: *"Use this if you sell the same product in different sizes, colors, or materials."*
3. On the very first variant matrix render in a browser session (tracked via `localStorage`), a small dismiss-once callout appears above the matrix: *"Edit any cell inline. Use `⌘D` to fill a value down. Removing a value removes its variants."* Dismissible with a single close action; never shown again on that machine.

#### 13.5.11 · Zero-products empty state copy (new, §7.2 addendum)

Draft copy committed to the spec so M7a doesn't block on brand writing:

- **Headline** (Source Serif 4): *"Your catalog starts here."*
- **Body** (Source Sans 3 muted): *"Add your first product — photos, variants, stock, and pricing all in one place. You can keep things simple or add every detail. Nothing goes live until you're ready."*
- **CTA** (moss primary): *"Add your first product"*

#### 13.5.12 · Color tokens and type scale (§7 addendum)

- Low-stock indicator in the list uses **`--signal`** (the editorial vermillion token), never a raw hex. Bound in `StatusDot`'s `tone="signal"` variant.
- Summary line (`42 products · 3 drafts · 2 archived`) renders in **Source Sans 3** at body size with `--ink-500` muted. Not serif. Never mistaken for the eyebrow treatment.
- Numerals in the variant matrix and price columns use Source Serif 4 tabular figures if the loaded weight supports them; otherwise Source Sans 3 tabular. M7a verifies.

### 13.6 · Security corrections (§ addendum; severity BLOCKING)

#### 13.6.1 · Rich-text sanitization

`products.description` stores TipTap output as sanitized HTML. Sanitization happens **on write** in the `product.Service.Create` / `Update` methods using `github.com/microcosm-cc/bluemonday` with a policy allowing only the editorial tags used by the admin editor (h1–h4, p, strong, em, ul, ol, li, blockquote, a with rel=nofollow, img — img is already a product media and is stripped from descriptions; only text-level tags allowed). The bluemonday policy is a committed artifact (`internal/product/sanitizer.go`), unit-tested, versioned.

The storefront reads and renders the already-sanitized field via `dangerouslySetInnerHTML` with confidence because every byte went through bluemonday before persistence. Storefront-side DOMPurify is a second layer (defense in depth) but not the authoritative boundary.

#### 13.6.2 · Upload size limits

Signed URLs include `x-goog-content-length-range: 0,10485760` (10 MiB max per object). Content type is constrained via `content-type` match. Only `image/jpeg`, `image/png`, `image/webp`, `image/avif` permitted. Videos explicitly refused in slice 1 even though `product_media.media_type` supports them.

GCS bucket has a lifecycle rule that auto-deletes any object under `tenants/*/products/media/` older than 24 hours with no matching DB row (see orphan sweep in §13.3).

#### 13.6.3 · Request body caps

Gin router sets `MaxMultipartMemory = 1MB` and `BodyLimit = 256KB` globally (products POST/PATCH is JSON, never multipart). Middleware rejects oversized bodies with a clean `413 payload_too_large` error envelope before any handler code runs.

#### 13.6.4 · Rate limiting

Admin routes: 60 requests/minute per `(tenant_id, user_id)`, burst 10. Storefront routes: 120 requests/minute per client IP, burst 30. Implemented via `go-shared/middleware/ratelimit` with an in-memory token bucket (single-replica for slice 1; a Redis backend is a slice-2 upgrade when the service goes multi-replica). Rate-limit responses carry `Retry-After`.

#### 13.6.5 · Structured mutation logging (observability floor)

Every admin mutation handler logs on entry and exit at INFO with: `request_id`, `user_id`, `tenant_id`, `store_id`, `operation` (`product.create` / `product.update` / etc.), `product_id`, `duration_ms`, `status`. This is the minimum observability floor for shipping to prod before metrics/tracing land. No PII in logs; titles and descriptions are hashed if included at all.

### 13.7 · Testing corrections (§9 addendum)

- **Pure validators** in §9.1 unit tests take primitive inputs only (no DB-backed dependencies). Anything that needs a store-currency lookup or a category ownership check runs at the service-integration layer in §9.3. The "no mocks" rule is preserved by drawing the boundary at the function signature: pure functions get unit tests; functions that read state get integration tests.
- **Concurrent-create test** (§9.4) is explicitly: 10 goroutines POST the same handle to the same store simultaneously; assert exactly one returns 201 and the other nine return `handle_taken`.
- **FGA test container version is pinned** (`openfga/openfga:v1.5.4` or the exact version `platform-api` currently uses; M4 reads `platform-api`'s CI config and matches).
- **Postgres test DB setup reuses `platform-api`'s `pkg/testdb`** (confirmed to exist at `services/platform-api/pkg/testdb/testdb.go`). Marketplace-api imports it via a small `internal/test/db.go` wrapper. The `//go:build integration` tag from `platform-api` is reused. This resolves §11.1.
- **Coverage gate**: 80% aggregate across `internal/product/`, `internal/category/`, `internal/media/`, `internal/stores/`; 90% target on pure-logic files (`sanitizer.go`, matrix helpers, validators). Gate configured via `-coverpkg=./internal/product/...,./internal/category/...,./internal/media/...,./internal/stores/...` — `cmd/` entrypoints are explicitly excluded.
- **Query-count assertion** on `ListAdmin`: the repository must load an N-product list in a bounded number of SQL queries (≤5 total regardless of N). An integration test uses `pgx` query logging or a `gorm` plugin to count actual queries and fail the test on N+1.

### 13.8 · M1 infra prerequisites (supersedes §8 M1 exit criteria)

M1 now explicitly delivers:

1. `services/marketplace-api/` service binary and Dockerfile (as before)
2. `marketplace_db` + `marketplace_user` provisioned in dev Cloud SQL
3. **`tesserix-infra/k8s/apps/marketplace/marketplace-api/` Kustomize overlay** (new — Knative Service + ServiceAccount + Cloud SQL Auth Proxy sidecar, mirroring an existing `go-service` base from `tesserix-infra`)
4. **`ExternalSecret` manifest** referencing GCP Secret Manager secrets for `marketplace_db` password and `X-Storefront-Key`
5. **ArgoCD Application registration** so the service deploys via `argocd` like the rest of the fleet
6. Dev-cluster deployment running and returning 200 from `/health` at the allocated URL
7. `.github/workflows/ci.yml` in the new service repo (or path in monorepo) with pinned Postgres 15, FGA, `fake-gcs-server` versions

The rationale is in the tech-lead review: M5–M7 depend on a deployed environment; hitting infra for the first time at M5 is how projects slip. Better to pay M1's ~1 day of infra work up front.

### 13.9 · Milestone 7 breakdown risk (§8 M7c addendum)

M7c is the single riskiest milestone in the plan. Its scope:

- `MoneyInput` promotion
- `MediaGrid` with GCS signed-URL upload flow + progress + reorder + alt text + variant-attach
- `OptionsEditor` (up to 3 axes, max-enforcement, inline validation)
- `VariantMatrixEditor` with generation, preservation-by-option-value-id, inline edit, bulk edit, row grouping, fill-down, column freeze, 100-variant cap, dismiss-once callout

Realistic estimate for a single developer: **5–8 working days**. If M7c runs long, the cut list (in order of preference to cut):

1. **Cut** row grouping + column freeze (slice 2 polish; ship with flat matrix first)
2. **Cut** fill-down keyboard shortcut (slice 2 polish)
3. **Cut** variant-attach popover on media (media stays product-level for slice 1; variant-level images move to slice 2)
4. **Cut** alt-text popover UX (alt text becomes a second row of fields visible all the time; less polished but functional)

Everything before "Cut" is load-bearing for M7d and must stay.

### 13.10 · Definition of done — revised (supersedes §12)

Replace the "onboard ~10 products" item with test-countable gates:

- [ ] Three Playwright E2E journeys from §9.5 pass in CI
- [ ] Unit + repository + service + API integration tests green; coverage gate met
- [ ] `marketplace-api` deployed to dev via ArgoCD
- [ ] `/health`, `/ready` reachable on both admin (`:8080`) and storefront (`:8081`) ports
- [ ] Bluemonday sanitizer unit tests cover XSS corpus (OWASP 10 top payloads)
- [ ] Orphan GCS sweep job committed and runs in dev
- [ ] Rate-limit middleware wired, 429 responses verified
- [ ] Structured mutation logs visible in GCP Cloud Logging for all admin routes
- [ ] Tenant-hard-delete placeholder: one-line strategy documented in README (`async cleanup via slice-N event; products remain until then`)
- [ ] `stores` projection: pull-through refresh works; `platform-api` outage does not break stale reads
- [ ] Single-developer smoke test: create 3 simple products, 1 variant product (2×3 matrix), 1 copy-to-store, all end-to-end with no workarounds — documented as a manual checkpoint in the PR description

### 13.11 · Open questions closed

- **§11.1 — test helper reuse:** resolved. Use `platform-api/pkg/testdb` via a thin `internal/test/db.go` wrapper.
- **§11.2 — StoreMiddleware shape:** resolved in §13.1.4.
- **§11.3 — `X-Store-ID` trust boundary:** resolved in §13.1.2 (separate port + path-based slug + shared secret).
- **§11.4 — FGA model version migration:** moot after §13.1.1 (no marketplace-specific FGA types to version).
- **§11.5 — Source Serif 4 tabular figures:** open; verified in M7a. Fallback is Source Sans 3 tabular.

### 13.12 · Component reuse map adjustments (§7.10 addendum)

- **`StoreSwitcher`** is a new admin-only composition (not promoted). Composes `@tesserix/web` `select` with the editorial override, reads stores from the new local projection via `GET /api/v1/admin/stores`. Lives in `apps/admin/components/shell/StoreSwitcher.tsx`.
- **`StatusDot`, `PriceDisplay`, `MoneyInput`** — pre-promoted to `@repo/ui` in a dedicated commit **before M7a starts**, not mid-milestone. This avoids context-switching cost during the UI crunch.
- **`dialog` / `confirm-dialog`** — used only for hard-delete confirmations (product delete, category delete with cascade). Removed from copy-to-store, variant removal, unsaved-changes guard, category management.
- **`sheet` / `drawer`** — not used anywhere in slice 1. Category management is a page route.

### 13.13 · Risk register (supersedes §10)

| Risk | Mitigation |
|---|---|
| FGA + DB drift | Eliminated by §13.1.1 (no per-object tuples). |
| Storefront leaks draft products | Repository-level `ListPublished` + distinct DTO types + separate Gin engine + shared-secret header + leak tests at repo and API layer. |
| Cross-tenant storefront read via forged `X-Store-ID` | §13.1.2 — no trusted header; slug + local lookup. |
| Variant matrix regeneration loses data | Matrix diff by `option_value_id` tuple; 15+ unit tests; inline removal banner with Undo. |
| GCS orphan objects | Content-addressed keys + nightly sweep job + 24h lifecycle rule. |
| Connection pool starvation on db-f1-micro | `SetMaxOpenConns(4)`; no GCS calls inside tx; `stores` projection removes platform-api calls from hot path; query-count test on `ListAdmin`; bulk import restricted to serialized per-tenant path with advisory lock. |
| XSS via rich-text description | Bluemonday sanitize on write; committed policy; XSS corpus in tests. |
| Oversized uploads | `x-goog-content-length-range`; bucket lifecycle; per-route body caps. |
| Currency drift on stores.currency_code change | BEFORE UPDATE trigger on variants; maintenance path documented. |
| Tenant hard delete orphans data | Placeholder: async cleanup job deferred to slice N; explicitly documented, not silently ignored. |
| M7c schedule overrun | Explicit cut list in §13.9. |
| Multi-warehouse slice 2 requires variant schema rewrite | Eliminated — `variant_stock` table exists from day one with a single default-location row. |
| Orders/webhooks slice 2 require outbox rewrite | Eliminated — `outbox_events` and `idempotency_keys` exist from day one. |
| Copy-to-store currency confusion | Dedicated page with distinct currency-mismatch callout when source ≠ target; draft status gates publish. |
| Autosave-free explicit-save data loss | `beforeunload` browser guard; unsaved state visible in sticky bar; failed-save retry inline. |

---

*End of v1.1 revisions. v1 authored 2026-04-09; v1.1 revisions 2026-04-09 (same day, after four-reviewer pass).*

---

## 14 · Revisions after re-review (v1.1 → v1.2)

After the v1.1 pass, a second round dispatched **five** independent reviewers (spec reviewer, architect, UX specialist, tech lead, and a database architect added this round). All 13 v1 BLOCKING issues verified resolved. The net-new items below are the **correctness and production-readiness fixes** from round two. UX polish items (breakpoints, hit targets, readonly-vs-disabled nuance, expand-all toggle, returnUrl pattern) are tracked separately as an implementation TODO list consumed by `writing-plans`.

**§14 supersedes §§1–13 where they conflict.**

### 14.1 · Watermark bottleneck eliminated (supersedes §13.1.3, §13.3 step 10)

`stores.products_updated_watermark` is **removed** from the `stores` projection. Serializing every mutation through a row lock on a single `stores` row is a pool-starvation hazard on db-f1-micro and a deadlock risk under concurrent writes to the same store.

Replacement: a separate `store_watermarks` table **maintained asynchronously** by the outbox publisher (which now ships as part of slice 1 — see §14.6).

```sql
CREATE TABLE store_watermarks (
    store_id             uuid         PRIMARY KEY,
    products_updated_at  timestamptz  NOT NULL DEFAULT now()
);
```

Flow:

1. Product/variant/media/category mutations write an `outbox_events` row in the same tx as the data write (unchanged).
2. The slice-1 outbox publisher goroutine reads events in batches, and for each batch upserts the watermark via `INSERT ... ON CONFLICT (store_id) DO UPDATE SET products_updated_at = GREATEST(store_watermarks.products_updated_at, EXCLUDED.products_updated_at)`. No lock contention on hot writes; the upsert is per-batch, not per-mutation.
3. Storefront ETag reads from `store_watermarks.products_updated_at` for the requested store — accepts eventual consistency of up to a few seconds (the interval between publisher ticks). This is well within the `s-maxage=60` cache window; merchants editing a product see the update reflected on the storefront within the cache boundary anyway.

Correctness implication: the storefront cache can briefly serve a stale response for up to ~`publisher_tick_interval + s-maxage` seconds after a mutation. This matches Shopify's publication latency and is an acceptable trade-off to eliminate the hot row.

### 14.2 · Currency change path redefined (supersedes §13.2.4)

The `BEFORE UPDATE` trigger on `product_variants` that reads `stores.currency_code` from the marketplace-api projection is **removed**. It created three problems: (a) read during an update acquires extra lock ordering overhead, (b) it duplicates enforcement logic already owned by the service layer, and (c) the 5-minute projection stale window means the trigger can enforce stale rules.

**Slice 1 decision: store currency changes are forbidden.** The platform-api `stores` table (authoritative source) rejects any `UPDATE` that mutates `currency_code` — a CHECK constraint at the platform-api layer or an explicit service-layer refusal. Merchants who need to change their store's currency must create a new store. This is a real constraint but an honest one: changing a live store's currency is a maintenance operation that touches every historical order, refund, coupon, and inventory ledger, not just the product catalog. Deferring it to a slice-N operation (with a dedicated migration runbook) is the correct call.

Consequence for marketplace-api: `product_variants.currency_code` is set once at variant creation from the value in `stores.currency_code` at that moment, and is effectively immutable. No trigger needed. Any code path attempting to change a variant's currency without also creating a new store is refused by the service layer with `currency_change_forbidden` error. A unit test asserts this.

### 14.3 · Partial unique index migration (supersedes §13.2.1)

The migration text needs to make the replacement explicit. v1.2 migration performs, in this exact order inside the transaction:

```sql
-- Inside 0001_products_initial.up.sql, AFTER the CREATE TABLE statements
-- that still declare the plain UNIQUE constraints (for readability of the DDL),
-- immediately drop them and replace with partial unique indexes:

ALTER TABLE categories       DROP CONSTRAINT categories_slug_per_store_unique;
ALTER TABLE products         DROP CONSTRAINT products_handle_per_store_unique;
ALTER TABLE product_variants DROP CONSTRAINT variants_sku_per_store_unique;

CREATE UNIQUE INDEX categories_slug_per_store_live_unique
    ON categories (store_id, slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX products_handle_per_store_live_unique
    ON products (store_id, handle) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX variants_sku_per_store_live_unique
    ON product_variants (store_id, sku) WHERE deleted_at IS NULL;
```

Repository-layer responsibility: catch `pgconn.PgError` with `Code == "23505"` (unique violation) on `INSERT` AND `UPDATE` paths (including any hypothetical "un-delete" that clears `deleted_at`) and return typed `handle_taken` / `sku_taken` / `slug_taken` errors with the conflicting value in `details`. A regression test exercises the un-delete path explicitly: soft-delete a row, insert a new live row with the same key, then attempt to clear `deleted_at` on the soft-deleted row and assert the typed error.

### 14.4 · Composite FK DDL placement (supersedes §13.2.3)

`products_id_store_unique` constraint moves into the original `CREATE TABLE products` DDL (not added via `ALTER TABLE` after the fact). The composite FK on `product_variants` references it directly in the original `CREATE TABLE product_variants` DDL. The migration reads top-to-bottom cleanly for future maintainers.

Down-migration drop order (explicitly enumerated):

```sql
DROP TABLE outbox_events;
DROP TABLE idempotency_keys;
DROP TABLE store_watermarks;
DROP TABLE variant_stock;
DROP TABLE product_categories;
DROP TABLE product_media;
DROP TABLE variant_option_values;
DROP TABLE product_variants;      -- must be before products due to composite FK
DROP TABLE product_option_values;
DROP TABLE product_options;
DROP TABLE products;
DROP TABLE categories;
DROP TABLE stores;
DROP FUNCTION IF EXISTS set_updated_at();
DROP FUNCTION IF EXISTS sync_variant_inventory();   -- see §14.5
```

`migrate up → down → up` cycle is a test fixture in M2.

### 14.5 · `variant_stock` dual-write eliminated via trigger (supersedes §13.2.6)

Service-layer maintenance of `product_variants.inventory_quantity` from `variant_stock.quantity` is replaced by a trigger, making `variant_stock` the single source of truth:

```sql
CREATE OR REPLACE FUNCTION sync_variant_inventory() RETURNS trigger AS $$
BEGIN
    -- Slice 1: one location per variant, so sum equals the single row's quantity.
    -- Slice 2 (multi-warehouse): this function becomes a SUM aggregate across
    -- all locations for the variant, or the denormalized column is dropped
    -- in favor of a view.
    UPDATE product_variants
    SET inventory_quantity = NEW.quantity
    WHERE id = NEW.variant_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER variant_stock_sync_insert
    AFTER INSERT ON variant_stock
    FOR EACH ROW EXECUTE FUNCTION sync_variant_inventory();

CREATE TRIGGER variant_stock_sync_update
    AFTER UPDATE OF quantity ON variant_stock
    FOR EACH ROW EXECUTE FUNCTION sync_variant_inventory();
```

Consequence: `product_variants.inventory_quantity` is **never written directly** by application code. The quick-update variant PATCH endpoint (`PATCH .../variants/:variantId`) updates `variant_stock.quantity` instead; the trigger takes care of the denormalized column. The service layer is simpler and correctness is enforced at the DB level.

Integration test: update `variant_stock`, read `product_variants`, assert the sum matches. Update `product_variants.inventory_quantity` directly (should not happen in real code, but the test guards future regressions), assert it does NOT affect `variant_stock` — making it clear which is source and which is derived.

### 14.6 · Outbox publisher ships in slice 1 (supersedes §13.2.7)

The publisher is no longer deferred to slice 2. Slice 1 ships a minimal, in-process publisher goroutine in `marketplace-api` with these responsibilities:

1. Poll `outbox_events WHERE published_at IS NULL` every 2 seconds, batch-fetch 100 rows via `SELECT ... FOR UPDATE SKIP LOCKED`
2. For each event, upsert into `store_watermarks` (§14.1)
3. Mark events as published (`UPDATE outbox_events SET published_at = now() WHERE id = ANY($1)`)
4. Pub/Sub delivery is **not** in slice 1 — events are written, consumed for watermark-bumping, and marked published. The shape is cemented; slice 2 replaces the watermark-only consumer with a real Pub/Sub publisher.

**Index shape correction:** the partial index on `outbox_events` now includes `tenant_id` up-front to support multi-tenant per-tenant publishers in slice 2 without a live-data reindex:

```sql
-- Supersedes the index in §13.2.7:
CREATE INDEX outbox_unpublished_idx
    ON outbox_events (tenant_id, created_at)
    WHERE published_at IS NULL;
```

The slice-1 single-goroutine publisher scans this index filtered on `WHERE published_at IS NULL`; Postgres uses the index for the ordering and the slice-2 per-tenant publisher uses it for the tenant filter without a `CREATE INDEX CONCURRENTLY` migration under live load.

**Operational guard:** a Cloud Monitoring alert fires if any `outbox_events` row has `published_at IS NULL AND created_at < now() - interval '7 days'`. The alert query is one line and runs against Cloud SQL directly, not through marketplace-api. This addresses the "unbounded growth" risk flagged by tech-lead review.

### 14.7 · `StoreMiddleware` serve-stale-on-error (supersedes §13.1.4)

Revised pseudocode that matches the DoD "platform-api outage does not break stale reads" claim:

```go
func StoreMiddleware(storeRepo stores.Repository, platformClient platform.Client, flight *singleflight.Group) gin.HandlerFunc {
    return func(c *gin.Context) {
        storeID := c.Param("storeId")
        tenantID := auth.TenantID(c)

        cached, cacheErr := storeRepo.GetByIDForTenant(c, storeID, tenantID)
        fresh := cacheErr == nil && !stores.IsStale(cached, 5*time.Minute)

        if fresh {
            c.Set("store", cached)
            c.Next()
            return
        }

        // Stale or missing → coalesce concurrent refreshes for the same key
        result, err, _ := flight.Do("store:"+storeID, func() (interface{}, error) {
            return refreshFromPlatform(c, platformClient, storeRepo, storeID, tenantID)
        })

        switch {
        case err == nil && result != nil:
            // Refresh succeeded
            c.Set("store", result.(*stores.Store))
            c.Next()
        case cacheErr == nil && cached != nil && time.Since(cached.SyncedAt) < 24*time.Hour:
            // Refresh failed but we have a cached row under 24h — serve stale with warning
            log.Warn(c, "serving stale store projection",
                "store_id", storeID, "synced_at", cached.SyncedAt, "refresh_err", err)
            c.Set("store", cached)
            c.Set("store_stale", true)
            c.Next()
        default:
            // No cache at all, OR cache older than 24h, OR cached row for wrong tenant
            apperrors.Respond(c, apperrors.NotFound("store"))
            c.Abort()
        }
    }
}
```

`singleflight.Group` (`golang.org/x/sync/singleflight`) prevents refresh stampedes — 50 concurrent requests for the same expired store key result in exactly one refresh call to platform-api. The stale-but-cached-within-24h window means a platform-api outage degrades gracefully: admin surfaces keep working with the last-known store metadata, only logging a warning.

DoD clause (§13.10) "platform-api outage does not break stale reads" is now realized by the code.

### 14.8 · Knative two-port / two-engine deployment shape (supersedes §13.1.2, §13.8)

Knative Services expose a single `containerPort` per revision. The two-engine design is implemented as **two Knative Services deployed from the same image**, differing only in env and VirtualService routing:

- `marketplace-api-admin` — env `MODE=admin`, the binary starts only the admin Gin engine on `:8080`, storefront engine is not constructed. VirtualService routes `/api/v1/admin/*` to this service.
- `marketplace-api-storefront` — env `MODE=storefront`, the binary starts only the storefront Gin engine on `:8080`, admin engine is not constructed. VirtualService routes `/api/v1/storefront/*` to this service.

One image, two Knative Services, deployed together from the same Kustomize overlay. M1 infra work now explicitly delivers both manifests. Scale-to-zero applies independently per service — the admin service doesn't need to stay warm when only storefront traffic flows, and vice versa.

Consequence: the "single binary" claim from §3.1 is technically two processes in prod, but the same Go code path and the same image. Local dev still runs a single process with both engines for convenience (gated by `MODE=both`).

**Rate limiter:** `marketplace-api-admin` pins `autoscaling.knative.dev/maxScale: "1"` in its Knative manifest for slice 1. This is the cheaper fix for T1: admin traffic is low-volume (merchants, not shoppers) and a single replica is sufficient. The in-memory token bucket's per-replica-doubling problem is sidestepped. Storefront (`marketplace-api-storefront`) scales normally since storefront rate limits are per-client-IP and N × limit under scale-out is acceptable (the per-IP bucket is not shared but the IP itself isn't under attack from the replica count).

Slice 2 task: Redis-backed rate limiter, unpin `maxScale`. Tracked explicitly in the DoD.

### 14.9 · Pre-tx GCS HEAD validates size + content-type (supersedes §13.3 step 2)

The pre-tx GCS verification is not just `exists?`. Full check:

```go
attrs, err := obj.Attrs(ctx)
if err != nil { return ErrUploadNotFound }
if attrs.Size > 10*1024*1024 { return ErrPayloadTooLarge }
if !slices.Contains(allowedImageTypes, attrs.ContentType) { return ErrUnsupportedMediaType }
```

Closes the gap where a client bypassing the signed URL could upload a larger or wrong-typed file. Defense-in-depth alongside the signed-URL `x-goog-content-length-range` constraint.

### 14.10 · Orphan sweep + bucket lifecycle: asymmetric TTLs (supersedes §13.6.2)

- GCS bucket lifecycle rule: delete unreferenced objects under `tenants/*/products/media/` **older than 48 hours**
- Nightly sweep job (CronJob): deletes orphans older than **24 hours**

The sweep is the primary GC path; the lifecycle rule is the backstop when the sweep fails. The 24h gap prevents the race where a legitimate slow upload (client computes sha256, uploads, waits 23h, finalizes) gets swept out from under the finalize request. Slow uploads have up to 24h before the sweep touches them, and up to 48h before the lifecycle rule does.

**Sweep job operational requirements** (new):

1. Exit with non-zero status on any error (Cloud Monitoring alerts on non-zero exit)
2. Emit a Cloud Logging INFO log for each deleted object with `storage_key`, `age_hours`, `bytes_freed`
3. Always performs `SELECT count(*) FROM product_media WHERE storage_key = $1` before deleting a GCS object — never deletes without a DB join check
4. Dead-man-switch alert: if no successful run in the last 48 hours, Cloud Monitoring pages the on-call (slice 1: the solo developer's email)
5. Job also runs `DELETE FROM idempotency_keys WHERE expires_at < now()` as a bonus step — one CronJob, two cleanup duties

### 14.11 · `products.store_id` local FK (supersedes §4 schema)

Now that `stores` is a local projection table in marketplace-api, `products.store_id` gets a real FK:

```sql
ALTER TABLE products
    ADD CONSTRAINT products_store_fk
        FOREIGN KEY (store_id)
        REFERENCES stores(id)
        ON DELETE RESTRICT;
```

Prevents products from referencing a store row that was never synced via `StoreMiddleware`. Combined with the middleware's upsert-before-write guarantee, creates a clean invariant: every product's store exists in the local projection.

Same FK applied to `categories.store_id`, `product_variants.store_id`, and `store_watermarks.store_id`.

### 14.12 · Content-addressed delete semantics (supersedes §13.3 step 8)

Explicit documentation of the in-tenant delete path:

- Deleting a product (soft delete) does **not** delete any GCS objects.
- Deleting a product hard (slice 1 supports soft only; future feature) removes `product_media` rows via `ON DELETE CASCADE`.
- The **only** GCS-delete code path is the nightly sweep job (§14.10), which checks `product_media` refcount by `storage_key` before deleting.
- Cross-tenant deduplication is intentionally absent. Each tenant has its own key namespace (`tenants/<tid>/products/media/<sha256>/...`), so the same bytes uploaded by two tenants live as two GCS objects. This is the correct trade-off — cross-tenant dedup creates data-isolation risks that outweigh the storage savings.

### 14.13 · Error code table additions (supersedes §13.4)

Add to the table in §13.4:

| Code | HTTP | Meaning |
|---|---|---|
| `payload_too_large` | 413 | JSON body exceeds 256 KB, multipart exceeds 1 MB, or an uploaded object exceeds 10 MiB |
| `unsupported_media_type` | 415 | Uploaded object has a content-type not in the allowlist |
| `rate_limited` | 429 | Admin (60/min per user) or storefront (120/min per IP) bucket exhausted; `Retry-After` header set |
| `currency_change_forbidden` | 409 | Attempted to mutate a store or variant currency — unsupported in slice 1 |
| `slug_taken` | 409 | `(store_id, slug)` collision on categories; `details.suggested` has an alternative |

All additions have at least one test producing them.

### 14.14 · Bluemonday policy versioning (supersedes §13.6.1)

Policy is pinned to a named constant in `internal/product/sanitizer.go`:

```go
const SanitizerPolicyVersion = 1

func policyV1() *bluemonday.Policy {
    p := bluemonday.NewPolicy()
    // ... exact allowed tags/attrs
    return p
}

var Policy = policyV1()
```

Stored HTML is never re-sanitized on read. If the policy is ever updated (e.g., `policyV2()` to allow a new tag or restrict an existing one), the policy constant bumps AND a migration job re-sanitizes every `products.description` by loading, applying `policyV2()`, and re-writing. The migration is explicitly part of any future PR that changes the policy — not optional.

README documents this as an invariant: "The bluemonday policy is append-only unless the change is accompanied by a re-sanitization migration." Unit test fixtures include a small XSS corpus (OWASP top-10 injection payloads) that the sanitizer must reduce to safe output.

### 14.15 · Structured error logging (§13.6.5 addendum)

Mutation handlers log on exit with the fields enumerated in §13.6.5. On error paths, logs additionally include: `error_code` (typed code from §13.4/§14.13), `error_message` (the `message` field of the error envelope, which is human-readable but PII-free by construction), and a `stack` field containing the wrapped call stack captured via `fmt.Errorf("%w", err)` or `errors.Wrap`. Level is INFO for success, WARN for expected errors (validation failures, 409s), ERROR for unexpected errors (pool exhaustion, platform-api unreachable, DB errors).

### 14.16 · Definition of done — v1.2 amendments (supersedes §13.10 in part)

Add these gates to §13.10:

- [ ] `outbox_events` publisher goroutine shipped; integration test asserts watermark is updated within 5 seconds of a mutation
- [ ] `store_watermarks` table populated correctly under load (query-count-asserted)
- [ ] `variant_stock` trigger present and tested: update stock → `product_variants.inventory_quantity` reflects
- [ ] `StoreMiddleware` serve-stale-on-error verified with a simulated platform-api outage in integration tests (20 stores, 10 requests per store, platform-api returns 503 — assert all requests succeed using cached rows, warning logs emitted)
- [ ] `marketplace-api-admin` Knative Service has `maxScale: 1` pinned with a comment linking to the slice-2 ticket for Redis rate limiter
- [ ] `marketplace-api-storefront` Knative Service deployed on the same image with `MODE=storefront` env
- [ ] GCS orphan sweep job + dead-man-switch Cloud Monitoring alert committed and tested
- [ ] `idempotency_keys` cleanup runs in the same CronJob and is tested
- [ ] Bluemonday policy pinned to version constant; XSS corpus test green
- [ ] Cloud Monitoring alert for `outbox_events` unpublished rows older than 7 days committed
- [ ] Platform-api `stores.currency_code` change forbidden at the source; regression test asserts

Replace the "smoke test" vibe item in §13.10 with: *"Four Playwright E2E journeys pass in CI"* — the fourth journey covers the simple catalog walkthrough previously described as a smoke test. The manual checkpoint becomes a CI-verifiable gate.

### 14.17 · Items explicitly deferred from v1.2 to implementation-time (tracked for `writing-plans`)

These items are **not** blockers for starting M1. They are polish and clarity refinements that are cheaper to resolve when the code is in front of us than to spec in advance. The implementation plan generated by `writing-plans` consumes this list as milestone TODOs:

- **UX refinements** (M7 scope): Variant matrix "Expand all / Collapse all" toggle; 1024–1279px breakpoint behavior for the left-margin anchor list; handle-suggestion rendered as distinct `Use this handle` button instead of prose `→`; staff read-only uses `readonly` for text/numeric inputs and `disabled` only for toggles/uploads; inline variant removal banner scrolls into view on insertion; two-click discard requires matching `href`; category management `?returnTo=<url>` pattern; bulk copy via POST body instead of query params; variant matrix first-render orientation sentence
- **Accessibility** (M7c): `aria-live` announcement for unsaved-changes state transitions; explicit acknowledgement in the spec that `beforeunload` is the single permitted browser-native modal exception
- **Schema DDL comments** (M2): comment on `products_published_requires_active` explaining the audit-timestamp intent; comment on the composite FK explaining the cascade interaction with soft delete
- **`locale` placeholder column** on the `stores` projection table (M2) — forward-compat for slice-N per-locale full-text search
- **`copy_source_product_id` rendering** (M7d): "Copied from (deleted product)" fallback when the source no longer exists
- **`ListAdmin` query-count gate** (M3): verify `≤5` against the actual repository design; raise to `≤7` if warranted, but assert whatever number is chosen
- **M7c proactive cut trigger** (execution): if M7c runs past day 6, proactively apply cut items 3 (variant-attach popover) and 4 (flat alt-text) without waiting for a slip
- **CI container startup budget** (M4): measure actual Postgres + FGA + fake-gcs-server cold-start overhead in GitHub Actions; if the 5-minute marketplace-api budget is tight, pre-warm via a dedicated "test-infra-ready" step
- **p99 latency + baseline load test** (slice 2 explicit commitment): before marketplace-api sees real merchant traffic, a baseline load test runs on a seeded 1000-product tenant catalog with representative read/write mix

### 14.18 · Risk register — v1.2 additions (supersedes §13.13 in part)

Add to §13.13:

| Risk | Mitigation |
|---|---|
| `stores` row lock serializing all mutations | §14.1: watermark moved to separate table, updated asynchronously by outbox publisher |
| Currency drift trigger + stale projection race | §14.2: currency changes forbidden in slice 1 |
| `variant_stock` / `inventory_quantity` dual-write drift | §14.5: DB trigger makes `variant_stock` single source of truth |
| Platform-api outage cascades to marketplace-api admin 404s | §14.7: serve-stale-on-error with singleflight, 24h cached-row ceiling |
| Knative scale-out multiplying in-memory rate limit | §14.8: admin service pins `maxScale: 1`; slice-2 Redis backend tracked as explicit DoD gate |
| Outbox table unbounded growth | §14.6: publisher ships in slice 1; Cloud Monitoring alert on 7-day unpublished row age |
| Orphan GCS object / in-flight upload race | §14.10: asymmetric 24h sweep / 48h lifecycle rule |
| Bluemonday policy drift between editor and storefront | §14.14: pinned policy version + mandatory re-sanitize migration on change |

---

*End of v1.2 revisions. v1.1 → v1.2 incorporates findings from the five-reviewer re-review round (spec reviewer, architect, UX specialist, tech lead, database architect). Items intentionally deferred to implementation time are enumerated in §14.17 for consumption by the `writing-plans` skill.*

