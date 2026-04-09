# Products M2 — Schema migration + domain models

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the complete slice-1 Products database (13 tables, partial unique indexes, composite FK, `variant_stock` trigger), delete the M1 scaffold migration, write GORM structs for every table, and prove correctness with a single round-trip integration test against real Postgres plus an `up → down → up` migration cycle test.

**Architecture:** One monolithic migration `000001_products_initial.{up,down}.sql` that replaces M1's no-op scaffold. Schema is copy-paste from the spec's §4 + the §14.x revisions. GORM structs live per-domain: `stores/`, `outbox/`, `idempotency/`, `category/`, `product/`. Zero business logic — M3 handles services, M5 handles HTTP. This milestone is the cemented data foundation everything else is built on.

**Tech Stack:** Go 1.26, Postgres 15, GORM, golang-migrate, `decimal.Decimal` for money, `pgx` via GORM's postgres driver.

**Spec references:** `docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` — authoritative order: §14 → §13 → §§1–12.
- §4 — original schema DDL (amended by §14.x — treat §4 + §14 jointly)
- §13.2.1 — partial unique indexes
- §13.2.3 — composite FK
- §13.2.6 — `variant_stock`
- §13.2.7 — outbox + idempotency
- §13.2.8 — `copy_source_product_id`
- §14.1 — `store_watermarks`
- §14.3 — partial-unique migration details
- §14.4 — composite FK DDL placement
- §14.5 — `sync_variant_inventory` trigger
- §14.6 — outbox index `(tenant_id, created_at)`
- §14.11 — `products.store_id` FK to local projection
- §14.12 — delete semantics for media

**Out of scope for M2:** All service-layer code, OpenFGA, HTTP handlers, admin/storefront routes, CI changes, Docker changes, test fixtures beyond the single round-trip test. Those land in M3–M7.

---

## Decisions locked for this milestone

1. **Monolithic migration.** One file `000001_products_initial.up.sql` creates every table, every index, every trigger in a single transaction. Rationale: M1's scaffold sits at version 1; M2 replaces it in-place without renumbering; no partial-schema intermediate state.
2. **ExpectedSchemaVersion stays at 1.** The scaffold migration is deleted; the real migration takes slot 1. No code change in `migrations.go`.
3. **Migration filename uses 6-digit versioning** (`000001_products_initial.up.sql`) to match what M1's scaffold used and what the golang-migrate iofs source expects. The spec's §4 mentions `0001_products_initial.up.sql` (4-digit); we deviate to 6-digit here for consistency with the existing M1 migration numbering.
4. **No GORM AutoMigrate.** All schema changes go through committed SQL migrations. GORM structs are read-only descriptions of the schema written in SQL. Tested by round-trip: insert via GORM, read via GORM, deep-equality assert.
5. **Package layout.** New internal packages created by M2: `internal/stores/`, `internal/outbox/`, `internal/idempotency/`, `internal/category/`, `internal/product/`. Each owns its `models.go` and nothing else (no repositories, no services — those land in M3).
6. **Integration test uses `//go:build integration` tag** (matches platform-api's `pkg/testdb` pattern). Tests run only when `TEST_DATABASE_URL` is set; CI sets it against the compose postgres.
7. **Round-trip test is a single function** exercising the full product graph end-to-end. Per-table unit tests for individual fields are **not** in scope — the spec review flagged "don't write tests that pass even if the code is wrong"; a single integration test that hits every model at once is more valuable than 13 per-field tests.
8. **`.keep.sql` and `000001_init.*` scaffold files are deleted** in the same commit as the real `000001_products_initial.*` lands. Intermediate state is broken by design — never checked out between commits.

---

## File structure produced by M2

```
services/marketplace-api/
├── migrations/
│   ├── 000001_products_initial.up.sql     # REPLACES 000001_init.up.sql
│   └── 000001_products_initial.down.sql   # REPLACES 000001_init.down.sql
│   # DELETED: .keep.sql, .gitkeep, 000001_init.up.sql, 000001_init.down.sql
├── internal/
│   ├── stores/
│   │   └── models.go                       # Store, StoreWatermark
│   ├── outbox/
│   │   └── models.go                       # OutboxEvent
│   ├── idempotency/
│   │   └── models.go                       # IdempotencyKey
│   ├── category/
│   │   └── models.go                       # Category
│   └── product/
│       ├── models.go                       # Product, Option, OptionValue, Variant, VariantOptionValue, Media
│       └── models_integration_test.go      # round-trip test (//go:build integration)
└── migrations.go                           # UNCHANGED (ExpectedSchemaVersion = 1)
```

---

## Task decomposition

**8 tasks total.** Tasks 1–2 are the migration (the hardest, most cross-cutting). Tasks 3–7 are the GORM models. Task 8 is the round-trip test. One commit per task.

---

### Task 1: Delete M1 scaffold + land the real migration (up.sql)

**Files:**
- Delete: `services/marketplace-api/migrations/000001_init.up.sql`
- Delete: `services/marketplace-api/migrations/000001_init.down.sql`
- Delete: `services/marketplace-api/migrations/.keep.sql`
- Delete: `services/marketplace-api/migrations/.gitkeep`
- Create: `services/marketplace-api/migrations/000001_products_initial.up.sql`

- [ ] **Step 1.1: Delete the scaffold files**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
rm services/marketplace-api/migrations/000001_init.up.sql
rm services/marketplace-api/migrations/000001_init.down.sql
rm services/marketplace-api/migrations/.keep.sql
rm services/marketplace-api/migrations/.gitkeep
```

- [ ] **Step 1.2: Write `000001_products_initial.up.sql` — EXACTLY this content**

```sql
-- ============================================================
-- 000001 · Products, categories, variants, options, media
--          + stores projection, watermarks, outbox, idempotency
-- Marketplace API · initial slice-1 schema
-- ============================================================

BEGIN;

-- ------------------------------------------------------------
-- stores (local projection, lazy pull-through synced by
-- StoreMiddleware from platform-api — see spec §14.7)
-- ------------------------------------------------------------
CREATE TABLE stores (
    id            uuid          PRIMARY KEY,
    tenant_id     uuid          NOT NULL,
    slug          varchar(63)   NOT NULL,
    name          varchar(200)  NOT NULL,
    country_code  char(2)       NOT NULL,
    currency_code char(3)       NOT NULL,
    timezone      varchar(64)   NOT NULL,
    status        varchar(20)   NOT NULL,
    synced_at     timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT stores_slug_unique  UNIQUE (slug),
    CONSTRAINT stores_status_valid CHECK (status IN ('active','suspended','archived'))
);
CREATE INDEX stores_tenant_idx ON stores (tenant_id);

-- ------------------------------------------------------------
-- store_watermarks — async-updated by the outbox publisher
-- (see spec §14.1; no hot-row lock on the stores table)
-- ------------------------------------------------------------
CREATE TABLE store_watermarks (
    store_id            uuid        PRIMARY KEY REFERENCES stores(id) ON DELETE CASCADE,
    products_updated_at timestamptz NOT NULL DEFAULT now()
);

-- ------------------------------------------------------------
-- categories — per-store tree; denormalised tenant_id for fast
-- tenant-wide admin queries; store_id is the real scope
-- ------------------------------------------------------------
CREATE TABLE categories (
    id          uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid          NOT NULL,
    store_id    uuid          NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    parent_id   uuid          REFERENCES categories(id) ON DELETE RESTRICT,
    name        varchar(200)  NOT NULL,
    slug        varchar(200)  NOT NULL,
    description text,
    image_url   text,
    position    integer       NOT NULL DEFAULT 0,
    is_active   boolean       NOT NULL DEFAULT true,
    created_at  timestamptz   NOT NULL DEFAULT now(),
    updated_at  timestamptz   NOT NULL DEFAULT now(),
    deleted_at  timestamptz,

    CONSTRAINT categories_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT categories_slug_format    CHECK (slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$')
);
CREATE UNIQUE INDEX categories_slug_per_store_live_unique
    ON categories (store_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX categories_tenant_idx ON categories (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX categories_store_idx  ON categories (store_id)  WHERE deleted_at IS NULL;
CREATE INDEX categories_parent_idx ON categories (parent_id) WHERE deleted_at IS NULL;

-- ------------------------------------------------------------
-- products — catalog record; no money, no stock; composite
-- UNIQUE (id, store_id) exists so product_variants can enforce
-- store_id consistency via a composite FK (spec §14.4)
-- ------------------------------------------------------------
CREATE TABLE products (
    id                     uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              uuid         NOT NULL,
    store_id               uuid         NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    handle                 varchar(200) NOT NULL,
    title                  varchar(300) NOT NULL,
    description            text,
    status                 varchar(20)  NOT NULL DEFAULT 'draft',
    vendor_id              uuid,
    tags                   text[]       NOT NULL DEFAULT '{}',
    seo_title              varchar(300),
    seo_description        varchar(500),
    primary_category_id    uuid         REFERENCES categories(id) ON DELETE SET NULL,
    copy_source_product_id uuid,  -- no FK; source may be soft-deleted or cross-store (spec §13.2.8)
    published_at           timestamptz,
    created_by             uuid,
    updated_by             uuid,
    created_at             timestamptz  NOT NULL DEFAULT now(),
    updated_at             timestamptz  NOT NULL DEFAULT now(),
    deleted_at             timestamptz,

    CONSTRAINT products_id_store_unique UNIQUE (id, store_id),
    CONSTRAINT products_status_valid    CHECK (status IN ('draft','active','archived')),
    CONSTRAINT products_title_not_blank CHECK (length(trim(title)) > 0),
    CONSTRAINT products_handle_format   CHECK (handle ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    CONSTRAINT products_published_requires_active CHECK (
        (status = 'active' AND published_at IS NOT NULL) OR (status <> 'active')
    )
);
CREATE UNIQUE INDEX products_handle_per_store_live_unique
    ON products (store_id, handle) WHERE deleted_at IS NULL;
CREATE INDEX products_tenant_idx      ON products (tenant_id)             WHERE deleted_at IS NULL;
CREATE INDEX products_store_idx       ON products (store_id)              WHERE deleted_at IS NULL;
CREATE INDEX products_status_idx      ON products (store_id, status)      WHERE deleted_at IS NULL;
CREATE INDEX products_primary_cat_idx ON products (primary_category_id)  WHERE deleted_at IS NULL;
CREATE INDEX products_published_idx   ON products (store_id, published_at DESC)
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX products_search_idx ON products
    USING gin (to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(description,'')))
    WHERE deleted_at IS NULL;
CREATE INDEX products_tags_idx ON products USING gin (tags) WHERE deleted_at IS NULL;

-- ------------------------------------------------------------
-- product_options — option axes (max 3 per product, app layer)
-- ------------------------------------------------------------
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

-- ------------------------------------------------------------
-- product_option_values — values per axis
-- ------------------------------------------------------------
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

-- ------------------------------------------------------------
-- product_variants — where money and stock live (composite FK
-- to products per spec §14.4; no inventory non-negative check
-- since policy='continue' allows negative per spec §13.2.2)
-- ------------------------------------------------------------
CREATE TABLE product_variants (
    id                  uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id          uuid            NOT NULL,
    store_id            uuid            NOT NULL,   -- denormalised for composite FK
    sku                 varchar(100)    NOT NULL,
    barcode             varchar(100),
    price               numeric(12, 2)  NOT NULL,
    compare_at_price    numeric(12, 2),
    cost_price          numeric(12, 2),
    currency_code       char(3)         NOT NULL,   -- set once at create; immutable per spec §14.2
    weight_grams        integer,
    inventory_quantity  integer         NOT NULL DEFAULT 0, -- trigger-maintained from variant_stock
    inventory_policy    varchar(20)     NOT NULL DEFAULT 'deny',
    low_stock_threshold integer,
    position            integer         NOT NULL DEFAULT 0,
    created_at          timestamptz     NOT NULL DEFAULT now(),
    updated_at          timestamptz     NOT NULL DEFAULT now(),
    deleted_at          timestamptz,

    CONSTRAINT variants_product_store_fk FOREIGN KEY (product_id, store_id)
        REFERENCES products (id, store_id) ON DELETE CASCADE,
    CONSTRAINT variants_price_non_negative          CHECK (price >= 0),
    CONSTRAINT variants_compare_price_non_negative  CHECK (compare_at_price IS NULL OR compare_at_price >= 0),
    CONSTRAINT variants_cost_price_non_negative     CHECK (cost_price IS NULL OR cost_price >= 0),
    CONSTRAINT variants_inventory_policy_valid      CHECK (inventory_policy IN ('deny','continue')),
    CONSTRAINT variants_currency_format             CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT variants_compare_gte_price           CHECK (compare_at_price IS NULL OR compare_at_price >= price)
);
CREATE UNIQUE INDEX variants_sku_per_store_live_unique
    ON product_variants (store_id, sku) WHERE deleted_at IS NULL;
CREATE INDEX variants_product_idx   ON product_variants (product_id)        WHERE deleted_at IS NULL;
CREATE INDEX variants_low_stock_idx ON product_variants (store_id, product_id)
    WHERE deleted_at IS NULL
      AND low_stock_threshold IS NOT NULL
      AND inventory_quantity <= low_stock_threshold;

-- ------------------------------------------------------------
-- variant_option_values — join (variant × option_value)
-- ------------------------------------------------------------
CREATE TABLE variant_option_values (
    variant_id      uuid NOT NULL REFERENCES product_variants(id)      ON DELETE CASCADE,
    option_value_id uuid NOT NULL REFERENCES product_option_values(id) ON DELETE RESTRICT,
    PRIMARY KEY (variant_id, option_value_id)
);
CREATE INDEX variant_option_values_value_idx ON variant_option_values (option_value_id);

-- ------------------------------------------------------------
-- variant_stock — per-location stock (slice 1 = one default
-- location per variant; slice 2+ adds multi-warehouse). The
-- trigger below keeps product_variants.inventory_quantity in
-- sync so legacy queries don't break (spec §14.5)
-- ------------------------------------------------------------
CREATE TABLE variant_stock (
    variant_id  uuid        NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    location_id uuid        NOT NULL,
    quantity    integer     NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (variant_id, location_id)
);
CREATE INDEX variant_stock_location_idx ON variant_stock (location_id);

-- ------------------------------------------------------------
-- product_media — first-class media; content-addressed storage
-- key (spec §14.12); variant_id nullable for product-level media
-- ------------------------------------------------------------
CREATE TABLE product_media (
    id          uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  uuid         NOT NULL REFERENCES products(id)        ON DELETE CASCADE,
    variant_id  uuid                  REFERENCES product_variants(id) ON DELETE SET NULL,
    url         text         NOT NULL,
    storage_key text         NOT NULL,
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

-- ------------------------------------------------------------
-- product_categories — M:N
-- ------------------------------------------------------------
CREATE TABLE product_categories (
    product_id  uuid NOT NULL REFERENCES products(id)   ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    PRIMARY KEY (product_id, category_id)
);
CREATE INDEX product_categories_category_idx ON product_categories (category_id);

-- ------------------------------------------------------------
-- outbox_events — cemented write path for slice 2 publishers
-- (index shape includes tenant_id up front per spec §14.6)
-- ------------------------------------------------------------
CREATE TABLE outbox_events (
    id           uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid         NOT NULL,
    aggregate    varchar(64)  NOT NULL,
    aggregate_id uuid         NOT NULL,
    event_type   varchar(64)  NOT NULL,
    payload      jsonb        NOT NULL,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    published_at timestamptz,
    error        text
);
CREATE INDEX outbox_unpublished_idx
    ON outbox_events (tenant_id, created_at) WHERE published_at IS NULL;

-- ------------------------------------------------------------
-- idempotency_keys — cleanup via the nightly sweep job (slice 1)
-- ------------------------------------------------------------
CREATE TABLE idempotency_keys (
    key        varchar(255) PRIMARY KEY,
    tenant_id  uuid         NOT NULL,
    response   jsonb,
    created_at timestamptz  NOT NULL DEFAULT now(),
    expires_at timestamptz  NOT NULL
);
CREATE INDEX idempotency_expires_idx ON idempotency_keys (expires_at);

-- ------------------------------------------------------------
-- Triggers
-- ------------------------------------------------------------

-- Shared updated_at bumper
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER categories_set_updated_at BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER products_set_updated_at   BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER variants_set_updated_at   BEFORE UPDATE ON product_variants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- variant_stock → product_variants.inventory_quantity sync
-- (single source of truth is variant_stock; slice-2 multi-warehouse
--  changes this function to SUM across locations — see spec §14.5)
CREATE OR REPLACE FUNCTION sync_variant_inventory() RETURNS trigger AS $$
BEGIN
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

COMMIT;
```

- [ ] **Step 1.3: Commit Task 1 (scaffold deletion + up migration together)**

Do NOT commit yet — Task 2 adds the down migration in the same commit. Move to Task 2.

---

### Task 2: Write the `down.sql` and commit Task 1+2 together

**Files:**
- Create: `services/marketplace-api/migrations/000001_products_initial.down.sql`

- [ ] **Step 2.1: Write `000001_products_initial.down.sql` — EXACTLY this content**

Drop order is dictated by the FK chain — reverse-dependency: anything that REFERENCES a table must be dropped before the table itself.

```sql
-- ============================================================
-- 000001 · DOWN migration — reverse-dependency drop order
-- ============================================================

BEGIN;

-- Triggers first (drops before their tables is not strictly
-- required — DROP TABLE cascades triggers — but being explicit
-- makes the rollback order match the reader's mental model)
DROP TRIGGER IF EXISTS variant_stock_sync_update  ON variant_stock;
DROP TRIGGER IF EXISTS variant_stock_sync_insert  ON variant_stock;
DROP TRIGGER IF EXISTS variants_set_updated_at    ON product_variants;
DROP TRIGGER IF EXISTS products_set_updated_at    ON products;
DROP TRIGGER IF EXISTS categories_set_updated_at  ON categories;

-- Functions after triggers
DROP FUNCTION IF EXISTS sync_variant_inventory();
DROP FUNCTION IF EXISTS set_updated_at();

-- Tables in reverse-dependency order
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS product_categories;
DROP TABLE IF EXISTS product_media;
DROP TABLE IF EXISTS variant_stock;
DROP TABLE IF EXISTS variant_option_values;
DROP TABLE IF EXISTS product_variants;         -- composite FK to products(id, store_id)
DROP TABLE IF EXISTS product_option_values;
DROP TABLE IF EXISTS product_options;
DROP TABLE IF EXISTS products;                  -- products_id_store_unique drops with the table
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS store_watermarks;
DROP TABLE IF EXISTS stores;

COMMIT;
```

- [ ] **Step 2.2: Verify the build still passes**

```bash
cd services/marketplace-api
go build ./...
```

Expected: no errors. The `go:embed migrations/*.sql` directive now matches exactly two files (`000001_products_initial.up.sql` and `000001_products_initial.down.sql`).

- [ ] **Step 2.3: Verify `migrate up` runs cleanly against a fresh DB**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
docker compose -f infra/dev/docker-compose.yml up -d postgres

# Drop and recreate marketplace_db to clear the M1 scaffold state
docker compose -f infra/dev/docker-compose.yml exec -T postgres psql -U dev -d postgres <<'SQL'
DROP DATABASE IF EXISTS marketplace_db;
CREATE DATABASE marketplace_db;
GRANT ALL PRIVILEGES ON DATABASE marketplace_db TO dev;
SQL

# Run the migrate binary against the fresh DB
docker compose -f infra/dev/docker-compose.yml run --rm marketplace-api-migrate up
```

Expected output (the final line): `migrations applied` or similar golang-migrate success message. If `no change` appears, it means the tracking table already exists at version 1 — in that case run `docker compose run --rm marketplace-api-migrate down 1` then `up` to force-reapply. The DROP DATABASE step above should have prevented this.

- [ ] **Step 2.4: Verify every expected table exists**

```bash
docker compose -f infra/dev/docker-compose.yml exec -T postgres psql -U dev -d marketplace_db -c "\dt"
```

Expected rows in the output: `stores`, `store_watermarks`, `categories`, `products`, `product_options`, `product_option_values`, `product_variants`, `variant_option_values`, `variant_stock`, `product_media`, `product_categories`, `outbox_events`, `idempotency_keys`, `marketplace_db_schema_migrations` — **14 tables total** (13 schema + 1 tracking table).

- [ ] **Step 2.5: Verify the up→down→up cycle is clean**

```bash
docker compose -f infra/dev/docker-compose.yml run --rm marketplace-api-migrate down 1
docker compose -f infra/dev/docker-compose.yml exec -T postgres psql -U dev -d marketplace_db -c "\dt"
```

Expected after down: only `marketplace_db_schema_migrations` remains (13 tables dropped cleanly).

```bash
docker compose -f infra/dev/docker-compose.yml run --rm marketplace-api-migrate up
docker compose -f infra/dev/docker-compose.yml exec -T postgres psql -U dev -d marketplace_db -c "\dt"
```

Expected after second up: all 14 tables present again. This verifies the down migration's drop order is correct.

- [ ] **Step 2.6: Commit Tasks 1 and 2 together**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add -A services/marketplace-api/migrations
git status  # Verify only migration files are staged
git commit -m "feat(marketplace-api): initial products schema — replaces M1 scaffold"
```

The `git add -A` picks up both the deletions (scaffold files) and the additions (real migration) in one staged set.

---

### Task 3: `internal/stores` GORM models

**Files:**
- Create: `services/marketplace-api/internal/stores/models.go`

- [ ] **Step 3.1: Write `internal/stores/models.go` — EXACTLY this content**

```go
// Package stores holds the local read-only projection of the authoritative
// store table owned by platform-api. Populated via lazy pull-through from
// StoreMiddleware (see spec §14.7). Never written outside of middleware.
package stores

import "time"

// Store is the marketplace-api view of a tenant's storefront.
// The canonical source is platform-api's stores table. This projection
// exists so StoreMiddleware can look up store metadata without an HTTP
// round-trip on every admin request (db-f1-micro 5-conn pool).
type Store struct {
	ID           string    `gorm:"primaryKey;column:id;type:uuid"                          json:"id"`
	TenantID     string    `gorm:"column:tenant_id;type:uuid;not null"                     json:"tenant_id"`
	Slug         string    `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"       json:"slug"`
	Name         string    `gorm:"column:name;type:varchar(200);not null"                  json:"name"`
	CountryCode  string    `gorm:"column:country_code;type:char(2);not null"               json:"country_code"`
	CurrencyCode string    `gorm:"column:currency_code;type:char(3);not null"              json:"currency_code"`
	Timezone     string    `gorm:"column:timezone;type:varchar(64);not null"               json:"timezone"`
	Status       string    `gorm:"column:status;type:varchar(20);not null"                 json:"status"`
	SyncedAt     time.Time `gorm:"column:synced_at;not null;default:now()"                 json:"synced_at"`
}

func (Store) TableName() string { return "stores" }

// StoreWatermark is bumped asynchronously by the outbox publisher after
// any product/variant/media/category mutation. Storefront ETag reads
// from this table, not from stores itself — the separation eliminates
// the hot-row lock on the authoritative store row (spec §14.1).
type StoreWatermark struct {
	StoreID            string    `gorm:"primaryKey;column:store_id;type:uuid"             json:"store_id"`
	ProductsUpdatedAt  time.Time `gorm:"column:products_updated_at;not null;default:now()" json:"products_updated_at"`
}

func (StoreWatermark) TableName() string { return "store_watermarks" }

// Status constants match the CHECK constraint in migration 000001.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusArchived  = "archived"
)
```

- [ ] **Step 3.2: Verify build**

```bash
cd services/marketplace-api
go build ./internal/stores/...
```

Expected: no errors.

- [ ] **Step 3.3: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/stores
git commit -m "feat(marketplace-api): stores projection + watermark models"
```

---

### Task 4: `internal/outbox` + `internal/idempotency` GORM models

**Files:**
- Create: `services/marketplace-api/internal/outbox/models.go`
- Create: `services/marketplace-api/internal/idempotency/models.go`

- [ ] **Step 4.1: Write `internal/outbox/models.go` — EXACTLY this content**

```go
// Package outbox holds the OutboxEvent model. Events are written in the
// same transaction as the mutation that produces them (see spec §13.2.7).
// Slice 1's publisher reads these rows, upserts store_watermarks, and
// marks them published. Slice 2 adds real Pub/Sub delivery.
package outbox

import (
	"time"

	"gorm.io/datatypes"
)

// OutboxEvent represents one pending or published domain event.
// The index on (tenant_id, created_at) WHERE published_at IS NULL
// supports the publisher's SELECT … FOR UPDATE SKIP LOCKED pattern
// (spec §14.6).
type OutboxEvent struct {
	ID          string         `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID    string         `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	Aggregate   string         `gorm:"column:aggregate;type:varchar(64);not null"               json:"aggregate"`
	AggregateID string         `gorm:"column:aggregate_id;type:uuid;not null"                   json:"aggregate_id"`
	EventType   string         `gorm:"column:event_type;type:varchar(64);not null"              json:"event_type"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb;not null"                       json:"payload"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	PublishedAt *time.Time     `gorm:"column:published_at"                                      json:"published_at,omitempty"`
	Error       *string        `gorm:"column:error;type:text"                                   json:"error,omitempty"`
}

func (OutboxEvent) TableName() string { return "outbox_events" }

// Aggregate constants used by producers.
const (
	AggregateProduct  = "product"
	AggregateCategory = "category"
	AggregateMedia    = "media"
)

// EventType constants.
const (
	EventProductCreated   = "product.created"
	EventProductUpdated   = "product.updated"
	EventProductDeleted   = "product.deleted"
	EventCategoryCreated  = "category.created"
	EventCategoryUpdated  = "category.updated"
	EventCategoryDeleted  = "category.deleted"
)
```

If `gorm.io/datatypes` is not already in `go.mod`, run `go get gorm.io/datatypes` inside `services/marketplace-api/` before building. `datatypes.JSON` is the canonical GORM wrapper for Postgres `jsonb` columns.

- [ ] **Step 4.2: Write `internal/idempotency/models.go` — EXACTLY this content**

```go
// Package idempotency holds the IdempotencyKey model. Cleanup of expired
// rows happens in the nightly sweep job (spec §14.10 — same CronJob as
// orphan GCS sweep). This package does not own the cleanup job itself,
// only the row shape.
package idempotency

import (
	"time"

	"gorm.io/datatypes"
)

// IdempotencyKey stores a previously-seen request key plus its response,
// keyed by an Idempotency-Key header value (see spec §13.2.7).
type IdempotencyKey struct {
	Key       string         `gorm:"primaryKey;column:key;type:varchar(255)" json:"key"`
	TenantID  string         `gorm:"column:tenant_id;type:uuid;not null"     json:"tenant_id"`
	Response  datatypes.JSON `gorm:"column:response;type:jsonb"              json:"response,omitempty"`
	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	ExpiresAt time.Time      `gorm:"column:expires_at;not null"              json:"expires_at"`
}

func (IdempotencyKey) TableName() string { return "idempotency_keys" }
```

- [ ] **Step 4.3: Verify build**

```bash
cd services/marketplace-api
go mod tidy
go build ./internal/outbox/... ./internal/idempotency/...
```

Include `go.mod` and `go.sum` in the commit if `datatypes` was a new dependency.

- [ ] **Step 4.4: Commit**

`gorm.io/datatypes` is a brand-new direct dependency (it is NOT a transitive of anything M1 landed). `go.mod` and `go.sum` MUST be in the commit — do not make them optional. Verify with `git status` before committing: both files should appear as modified.

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/outbox services/marketplace-api/internal/idempotency
git add services/marketplace-api/go.mod services/marketplace-api/go.sum
git status --short  # Expect: M internal/outbox/..., M internal/idempotency/..., M go.mod, M go.sum
git commit -m "feat(marketplace-api): outbox + idempotency key models"
```

---

### Task 5: `internal/category` GORM models

**Files:**
- Create: `services/marketplace-api/internal/category/models.go`

- [ ] **Step 5.1: Write `internal/category/models.go` — EXACTLY this content**

```go
// Package category owns the Category tree model. Categories are per-store
// (the real scope) with tenant_id denormalised for fast tenant-wide admin
// queries. Soft-delete via deleted_at; partial unique index on (store_id,
// slug) WHERE deleted_at IS NULL so deleted slugs are reusable.
package category

import "time"

// Category is one node in the per-store category tree.
type Category struct {
	ID          string     `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID    string     `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID     string     `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	ParentID    *string    `gorm:"column:parent_id;type:uuid"                               json:"parent_id,omitempty"`
	Name        string     `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	Slug        string     `gorm:"column:slug;type:varchar(200);not null"                   json:"slug"`
	Description *string    `gorm:"column:description;type:text"                             json:"description,omitempty"`
	ImageURL    *string    `gorm:"column:image_url;type:text"                               json:"image_url,omitempty"`
	Position    int        `gorm:"column:position;not null;default:0"                       json:"position"`
	IsActive    bool       `gorm:"column:is_active;not null;default:true"                   json:"is_active"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at;index"                                  json:"deleted_at,omitempty"`
}

func (Category) TableName() string { return "categories" }
```

- [ ] **Step 5.2: Verify build**

```bash
cd services/marketplace-api
go build ./internal/category/...
```

- [ ] **Step 5.3: Commit**

```bash
git add services/marketplace-api/internal/category
git commit -m "feat(marketplace-api): category model"
```

---

### Task 6: `internal/product` GORM models (the big one)

**Files:**
- Create: `services/marketplace-api/internal/product/models.go`

- [ ] **Step 6.1: Write `internal/product/models.go` — EXACTLY this content**

```go
// Package product owns the Product aggregate and its child models:
// Option, OptionValue, Variant, VariantOptionValue, Media. Categories
// and Stores are separate packages; Product references their IDs only.
//
// Design notes:
//   - Product has no money or stock — those live on Variant.
//   - Variant has a composite FK (product_id, store_id) → products
//     (id, store_id) so store_id can never drift (spec §14.4).
//   - Variant.InventoryQuantity is maintained by a DB trigger on
//     variant_stock (spec §14.5). Do NOT write to it directly; write
//     to variant_stock and let the trigger propagate.
//   - Media.StorageKey is content-addressed (sha256-prefixed path);
//     refcount is a `count(*) on storage_key` query — the same object
//     may be referenced by multiple product_media rows after copy-to-store.
package product

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/lib/pq"
)

// Status constants match the CHECK constraint in migration 000001.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// InventoryPolicy constants.
const (
	InventoryPolicyDeny     = "deny"     // reject orders when stock hits 0
	InventoryPolicyContinue = "continue" // accept backorders, allow negative stock
)

// MediaType constants.
const (
	MediaTypeImage = "image"
	MediaTypeVideo = "video"
)

// Product is the catalog record. No prices, no stock — see Variant.
type Product struct {
	ID                  string     `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID            string     `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID             string     `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	Handle              string     `gorm:"column:handle;type:varchar(200);not null"                 json:"handle"`
	Title               string     `gorm:"column:title;type:varchar(300);not null"                  json:"title"`
	Description         *string    `gorm:"column:description;type:text"                             json:"description,omitempty"`
	Status              string     `gorm:"column:status;type:varchar(20);not null;default:draft"    json:"status"`
	VendorID            *string    `gorm:"column:vendor_id;type:uuid"                               json:"vendor_id,omitempty"`
	Tags                pq.StringArray `gorm:"column:tags;type:text[];not null;default:'{}'"        json:"tags"`
	SEOTitle            *string    `gorm:"column:seo_title;type:varchar(300)"                       json:"seo_title,omitempty"`
	SEODescription      *string    `gorm:"column:seo_description;type:varchar(500)"                 json:"seo_description,omitempty"`
	PrimaryCategoryID   *string    `gorm:"column:primary_category_id;type:uuid"                     json:"primary_category_id,omitempty"`
	CopySourceProductID *string    `gorm:"column:copy_source_product_id;type:uuid"                  json:"copy_source_product_id,omitempty"`
	PublishedAt         *time.Time `gorm:"column:published_at"                                      json:"published_at,omitempty"`
	CreatedBy           *string    `gorm:"column:created_by;type:uuid"                              json:"created_by,omitempty"`
	UpdatedBy           *string    `gorm:"column:updated_by;type:uuid"                              json:"updated_by,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
	DeletedAt           *time.Time `gorm:"column:deleted_at;index"                                  json:"deleted_at,omitempty"`

	// Eager-loaded relations (optional; populated via Preload)
	Options  []Option   `gorm:"foreignKey:ProductID" json:"options,omitempty"`
	Variants []Variant  `gorm:"foreignKey:ProductID" json:"variants,omitempty"`
	Media    []Media    `gorm:"foreignKey:ProductID" json:"media,omitempty"`
}

func (Product) TableName() string { return "products" }

// Option is an option axis for a product (Size, Color, Material).
// Max 3 per product, enforced in the service layer.
type Option struct {
	ID        string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	ProductID string    `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	Name      string    `gorm:"column:name;type:varchar(100);not null"                   json:"name"`
	Position  int       `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`

	Values []OptionValue `gorm:"foreignKey:OptionID" json:"values,omitempty"`
}

func (Option) TableName() string { return "product_options" }

// OptionValue is one value on an option axis.
type OptionValue struct {
	ID        string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	OptionID  string    `gorm:"column:option_id;type:uuid;not null"                      json:"option_id"`
	Value     string    `gorm:"column:value;type:varchar(200);not null"                  json:"value"`
	Position  int       `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (OptionValue) TableName() string { return "product_option_values" }

// Variant is the sellable unit — where money and stock live.
// InventoryQuantity is trigger-maintained from variant_stock; do not write
// to it directly.
type Variant struct {
	ID                string          `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	ProductID         string          `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	StoreID           string          `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	SKU               string          `gorm:"column:sku;type:varchar(100);not null"                    json:"sku"`
	Barcode           *string         `gorm:"column:barcode;type:varchar(100)"                         json:"barcode,omitempty"`
	Price             decimal.Decimal `gorm:"column:price;type:numeric(12,2);not null"                 json:"price"`
	CompareAtPrice    *decimal.Decimal `gorm:"column:compare_at_price;type:numeric(12,2)"              json:"compare_at_price,omitempty"`
	CostPrice         *decimal.Decimal `gorm:"column:cost_price;type:numeric(12,2)"                    json:"cost_price,omitempty"`
	CurrencyCode      string          `gorm:"column:currency_code;type:char(3);not null"               json:"currency_code"`
	WeightGrams       *int            `gorm:"column:weight_grams"                                      json:"weight_grams,omitempty"`
	InventoryQuantity int             `gorm:"column:inventory_quantity;not null;default:0"             json:"inventory_quantity"`
	InventoryPolicy   string          `gorm:"column:inventory_policy;type:varchar(20);not null;default:deny" json:"inventory_policy"`
	LowStockThreshold *int            `gorm:"column:low_stock_threshold"                               json:"low_stock_threshold,omitempty"`
	Position          int             `gorm:"column:position;not null;default:0"                       json:"position"`
	CreatedAt         time.Time       `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt         time.Time       `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
	DeletedAt         *time.Time      `gorm:"column:deleted_at;index"                                  json:"deleted_at,omitempty"`

	OptionValueLinks []VariantOptionValue `gorm:"foreignKey:VariantID" json:"option_value_links,omitempty"`
}

func (Variant) TableName() string { return "product_variants" }

// VariantOptionValue joins a variant to one option value. The pair
// (variant_id, option_value_id) is the primary key.
type VariantOptionValue struct {
	VariantID     string `gorm:"primaryKey;column:variant_id;type:uuid"      json:"variant_id"`
	OptionValueID string `gorm:"primaryKey;column:option_value_id;type:uuid" json:"option_value_id"`
}

func (VariantOptionValue) TableName() string { return "variant_option_values" }

// Media is a product-level or variant-level media asset.
// StorageKey is content-addressed; refcount via count(*) on storage_key.
type Media struct {
	ID         string    `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	ProductID  string    `gorm:"column:product_id;type:uuid;not null"                     json:"product_id"`
	VariantID  *string   `gorm:"column:variant_id;type:uuid"                              json:"variant_id,omitempty"`
	URL        string    `gorm:"column:url;type:text;not null"                            json:"url"`
	StorageKey string    `gorm:"column:storage_key;type:text;not null"                    json:"storage_key"`
	Alt        *string   `gorm:"column:alt;type:varchar(300)"                             json:"alt,omitempty"`
	Position   int       `gorm:"column:position;not null;default:0"                       json:"position"`
	MediaType  string    `gorm:"column:media_type;type:varchar(20);not null;default:image" json:"media_type"`
	Width      *int      `gorm:"column:width"                                             json:"width,omitempty"`
	Height     *int      `gorm:"column:height"                                            json:"height,omitempty"`
	Bytes      *int64    `gorm:"column:bytes"                                             json:"bytes,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (Media) TableName() string { return "product_media" }

// ProductCategory joins a product to a category (M:N).
type ProductCategory struct {
	ProductID  string `gorm:"primaryKey;column:product_id;type:uuid"  json:"product_id"`
	CategoryID string `gorm:"primaryKey;column:category_id;type:uuid" json:"category_id"`
}

func (ProductCategory) TableName() string { return "product_categories" }

// VariantStock is the per-location stock row. Writing to this table
// triggers sync_variant_inventory() which updates the denormalised
// Variant.InventoryQuantity column. Slice 1 uses exactly one row per
// variant at DEFAULT_LOCATION_ID; slice 2+ extends to multi-warehouse.
//
// The composite primary key (variant_id, location_id) is declared
// explicitly so GORM does NOT try to add a RETURNING id clause on
// INSERT. Without the primaryKey tags on both fields, GORM would look
// for a single-column PK and fail.
type VariantStock struct {
	VariantID  string    `gorm:"primaryKey;column:variant_id;type:uuid"     json:"variant_id"`
	LocationID string    `gorm:"primaryKey;column:location_id;type:uuid"    json:"location_id"`
	Quantity   int       `gorm:"column:quantity;not null;default:0"          json:"quantity"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null;default:now()"    json:"updated_at"`
}

func (VariantStock) TableName() string { return "variant_stock" }
```

- [ ] **Step 6.2: Verify build**

```bash
cd services/marketplace-api
go get github.com/shopspring/decimal
go get github.com/lib/pq
go mod tidy
go build ./internal/product/...
```

`github.com/shopspring/decimal` and `github.com/lib/pq` are new direct dependencies (pq is for `pq.StringArray`, which is the canonical Postgres `text[]` wrapper for GORM). Include the updated `go.mod` / `go.sum` in the commit.

- [ ] **Step 6.3: Commit**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/product services/marketplace-api/go.mod services/marketplace-api/go.sum
git commit -m "feat(marketplace-api): product aggregate models (Product/Option/Variant/Media)"
```

---

### Task 7: Round-trip integration test

**Files:**
- Create: `services/marketplace-api/internal/product/models_integration_test.go`

- [ ] **Step 7.1: Write `models_integration_test.go` — EXACTLY this content**

```go
//go:build integration

package product_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestIntegration_FullProductGraph_RoundTrip inserts a complete slice-1
// product graph — store → category → product → 2 options → 6 option values
// → 6 variants → 12 variant_option_values → 3 media → 2 product_categories
// → variant_stock rows → and asserts every row round-trips via Preload,
// plus that the variant_stock trigger correctly maintains
// product_variants.inventory_quantity.
func TestIntegration_FullProductGraph_RoundTrip(t *testing.T) {
	tx := testdb.NewTx(t)

	// 1. Insert store projection row (normally populated by StoreMiddleware)
	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	store := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "acme-eu-" + uuid.NewString()[:8],
		Name:         "Acme EU",
		CountryCode:  "DE",
		CurrencyCode: "EUR",
		Timezone:     "Europe/Berlin",
		Status:       stores.StatusActive,
	}
	if err := tx.Create(store).Error; err != nil {
		t.Fatalf("insert store: %v", err)
	}

	// 2. Insert two categories (one root, one nested)
	rootCat := &category.Category{
		TenantID: tenantID,
		StoreID:  storeID,
		Name:     "Apparel",
		Slug:     "apparel",
		Position: 0,
		IsActive: true,
	}
	if err := tx.Create(rootCat).Error; err != nil {
		t.Fatalf("insert root category: %v", err)
	}
	shirtCat := &category.Category{
		TenantID: tenantID,
		StoreID:  storeID,
		ParentID: &rootCat.ID,
		Name:     "Shirts",
		Slug:     "shirts",
		Position: 0,
		IsActive: true,
	}
	if err := tx.Create(shirtCat).Error; err != nil {
		t.Fatalf("insert shirt category: %v", err)
	}

	// 3. Insert the product record
	prod := &product.Product{
		TenantID:          tenantID,
		StoreID:           storeID,
		Handle:            "linen-shirt",
		Title:             "Linen Shirt",
		Status:            product.StatusDraft,
		Tags:              []string{"summer", "linen"},
		PrimaryCategoryID: &shirtCat.ID,
	}
	if err := tx.Create(prod).Error; err != nil {
		t.Fatalf("insert product: %v", err)
	}

	// 4. Insert two options (Size, Color) with 3 and 2 values respectively
	sizeOpt := &product.Option{ProductID: prod.ID, Name: "Size", Position: 0}
	colorOpt := &product.Option{ProductID: prod.ID, Name: "Color", Position: 1}
	if err := tx.Create(sizeOpt).Error; err != nil {
		t.Fatalf("insert size option: %v", err)
	}
	if err := tx.Create(colorOpt).Error; err != nil {
		t.Fatalf("insert color option: %v", err)
	}

	sizeValues := []*product.OptionValue{
		{OptionID: sizeOpt.ID, Value: "S", Position: 0},
		{OptionID: sizeOpt.ID, Value: "M", Position: 1},
		{OptionID: sizeOpt.ID, Value: "L", Position: 2},
	}
	colorValues := []*product.OptionValue{
		{OptionID: colorOpt.ID, Value: "Sand", Position: 0},
		{OptionID: colorOpt.ID, Value: "Ink", Position: 1},
	}
	for _, v := range sizeValues {
		if err := tx.Create(v).Error; err != nil {
			t.Fatalf("insert size value %q: %v", v.Value, err)
		}
	}
	for _, v := range colorValues {
		if err := tx.Create(v).Error; err != nil {
			t.Fatalf("insert color value %q: %v", v.Value, err)
		}
	}

	// 5. Insert 6 variants (3 sizes × 2 colors) with variant_option_values joins
	//    Each variant gets a SKU and a price; inventory_quantity stays at 0 here
	//    (trigger populates it from variant_stock in step 7).
	variants := make([]*product.Variant, 0, 6)
	for si, sv := range sizeValues {
		for ci, cv := range colorValues {
			v := &product.Variant{
				ProductID:       prod.ID,
				StoreID:         storeID,
				SKU:             "LIN-" + sv.Value + "-" + cv.Value,
				Price:           decimal.NewFromFloat(89.00),
				CurrencyCode:    "EUR",
				InventoryPolicy: product.InventoryPolicyDeny,
				Position:        si*2 + ci,
			}
			if err := tx.Create(v).Error; err != nil {
				t.Fatalf("insert variant %s: %v", v.SKU, err)
			}
			if err := tx.Create(&product.VariantOptionValue{VariantID: v.ID, OptionValueID: sv.ID}).Error; err != nil {
				t.Fatalf("link variant %s to size value: %v", v.SKU, err)
			}
			if err := tx.Create(&product.VariantOptionValue{VariantID: v.ID, OptionValueID: cv.ID}).Error; err != nil {
				t.Fatalf("link variant %s to color value: %v", v.SKU, err)
			}
			variants = append(variants, v)
		}
	}
	if len(variants) != 6 {
		t.Fatalf("expected 6 variants, got %d", len(variants))
	}

	// 6. Insert 3 media rows
	mediaRows := []*product.Media{
		{ProductID: prod.ID, URL: "https://cdn.test/1.jpg", StorageKey: "tenants/" + tenantID + "/products/media/abc/1.jpg", Position: 0, MediaType: product.MediaTypeImage},
		{ProductID: prod.ID, URL: "https://cdn.test/2.jpg", StorageKey: "tenants/" + tenantID + "/products/media/def/2.jpg", Position: 1, MediaType: product.MediaTypeImage},
		{ProductID: prod.ID, VariantID: &variants[0].ID, URL: "https://cdn.test/3.jpg", StorageKey: "tenants/" + tenantID + "/products/media/ghi/3.jpg", Position: 2, MediaType: product.MediaTypeImage},
	}
	for _, m := range mediaRows {
		if err := tx.Create(m).Error; err != nil {
			t.Fatalf("insert media %s: %v", m.StorageKey, err)
		}
	}

	// 7. Insert variant_stock rows → trigger should sync inventory_quantity
	defaultLocation := "00000000-0000-0000-0000-000000000001"
	for i, v := range variants {
		stock := &product.VariantStock{
			VariantID:  v.ID,
			LocationID: defaultLocation,
			Quantity:   10 + i, // 10, 11, 12, 13, 14, 15
		}
		if err := tx.Create(stock).Error; err != nil {
			t.Fatalf("insert variant_stock for %s: %v", v.SKU, err)
		}
	}

	// 8. Insert two product_categories rows
	if err := tx.Create(&product.ProductCategory{ProductID: prod.ID, CategoryID: rootCat.ID}).Error; err != nil {
		t.Fatalf("link product to root category: %v", err)
	}
	if err := tx.Create(&product.ProductCategory{ProductID: prod.ID, CategoryID: shirtCat.ID}).Error; err != nil {
		t.Fatalf("link product to shirt category: %v", err)
	}

	// 9. Round-trip: read the product back with eager loads and verify
	var readProd product.Product
	err := tx.
		Preload("Options").
		Preload("Options.Values").
		Preload("Variants").
		Preload("Variants.OptionValueLinks").
		Preload("Media").
		First(&readProd, "id = ?", prod.ID).Error
	if err != nil {
		t.Fatalf("read product back: %v", err)
	}

	if readProd.Handle != "linen-shirt" {
		t.Errorf("handle = %q, want linen-shirt", readProd.Handle)
	}
	if len(readProd.Options) != 2 {
		t.Errorf("options len = %d, want 2", len(readProd.Options))
	}
	if len(readProd.Variants) != 6 {
		t.Errorf("variants len = %d, want 6", len(readProd.Variants))
	}
	if len(readProd.Media) != 3 {
		t.Errorf("media len = %d, want 3", len(readProd.Media))
	}
	if len(readProd.Tags) != 2 || readProd.Tags[0] != "summer" || readProd.Tags[1] != "linen" {
		t.Errorf("tags = %v, want [summer linen]", readProd.Tags)
	}

	// 10. Verify the variant_stock trigger populated inventory_quantity.
	// We inserted variants with quantities 10..15 via variant_stock; the
	// AFTER INSERT trigger on variant_stock should have updated
	// product_variants.inventory_quantity to match.
	qtyByID := map[string]int{}
	var refreshedVariants []product.Variant
	if err := tx.Where("product_id = ?", prod.ID).Find(&refreshedVariants).Error; err != nil {
		t.Fatalf("refetch variants: %v", err)
	}
	for _, v := range refreshedVariants {
		qtyByID[v.SKU] = v.InventoryQuantity
	}
	expected := map[string]int{
		"LIN-S-Sand": 10, "LIN-S-Ink": 11,
		"LIN-M-Sand": 12, "LIN-M-Ink": 13,
		"LIN-L-Sand": 14, "LIN-L-Ink": 15,
	}
	for sku, wantQty := range expected {
		if got := qtyByID[sku]; got != wantQty {
			t.Errorf("variant_stock trigger: %s inventory_quantity = %d, want %d", sku, got, wantQty)
		}
	}

	// 11. Update variant_stock → trigger should re-sync
	firstVariantID := variants[0].ID
	if err := tx.Exec(
		"UPDATE variant_stock SET quantity = ? WHERE variant_id = ? AND location_id = ?",
		99, firstVariantID, defaultLocation,
	).Error; err != nil {
		t.Fatalf("update variant_stock: %v", err)
	}
	var updatedVariant product.Variant
	if err := tx.First(&updatedVariant, "id = ?", firstVariantID).Error; err != nil {
		t.Fatalf("refetch first variant: %v", err)
	}
	if updatedVariant.InventoryQuantity != 99 {
		t.Errorf("after update trigger: inventory_quantity = %d, want 99", updatedVariant.InventoryQuantity)
	}
}

// TestIntegration_PartialUnique_SoftDelete verifies that the partial
// unique index on (store_id, handle) WHERE deleted_at IS NULL lets a
// new live row reuse a handle after the previous row is soft-deleted.
//
// Postgres aborts an entire transaction on any SQL error, so every
// expected-failure statement (the duplicate insert, the un-delete)
// MUST run inside its own SAVEPOINT. Rolling back the savepoint after
// the expected error leaves the outer tx healthy for the next step.
func TestIntegration_PartialUnique_SoftDelete(t *testing.T) {
	tx := testdb.NewTx(t)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	store := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "acme-soft-" + uuid.NewString()[:8],
		Name:         "Acme Soft",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
		Status:       stores.StatusActive,
	}
	if err := tx.Create(store).Error; err != nil {
		t.Fatalf("insert store: %v", err)
	}

	// Insert first product (real statement; stays committed inside the outer tx)
	p1 := &product.Product{
		TenantID: tenantID,
		StoreID:  storeID,
		Handle:   "silk-scarf",
		Title:    "Silk Scarf",
		Status:   product.StatusDraft,
	}
	if err := tx.Create(p1).Error; err != nil {
		t.Fatalf("insert first product: %v", err)
	}

	// Expected-failure #1: duplicate live handle → must fail with unique
	// violation. Wrap in a savepoint so the outer tx survives the error.
	if err := tx.SavePoint("before_dup_insert").Error; err != nil {
		t.Fatalf("savepoint before_dup_insert: %v", err)
	}
	p2 := &product.Product{
		TenantID: tenantID,
		StoreID:  storeID,
		Handle:   "silk-scarf",
		Title:    "Silk Scarf (v2)",
		Status:   product.StatusDraft,
	}
	dupErr := tx.Create(p2).Error
	if dupErr == nil {
		t.Fatal("expected unique violation on duplicate live handle, got nil")
	}
	if err := tx.RollbackTo("before_dup_insert").Error; err != nil {
		t.Fatalf("rollback to before_dup_insert: %v", err)
	}

	// Soft-delete the first product (real statement)
	now := time.Now()
	if err := tx.Model(&product.Product{}).Where("id = ?", p1.ID).Update("deleted_at", now).Error; err != nil {
		t.Fatalf("soft-delete first product: %v", err)
	}

	// Now inserting the duplicate should succeed — p1 is no longer "live"
	p3 := &product.Product{
		TenantID: tenantID,
		StoreID:  storeID,
		Handle:   "silk-scarf",
		Title:    "Silk Scarf (v3)",
		Status:   product.StatusDraft,
	}
	if err := tx.Create(p3).Error; err != nil {
		t.Fatalf("insert after soft-delete: %v", err)
	}

	// Expected-failure #2: un-deleting p1 while p3 is live must fail
	// (two live rows with the same handle would violate the partial
	// unique index). Savepoint again.
	if err := tx.SavePoint("before_undelete").Error; err != nil {
		t.Fatalf("savepoint before_undelete: %v", err)
	}
	undeleteErr := tx.Model(&product.Product{}).Where("id = ?", p1.ID).
		Update("deleted_at", gorm.Expr("NULL")).Error
	if undeleteErr == nil {
		t.Fatal("expected unique violation on un-delete with conflicting live row, got nil")
	}
	if err := tx.RollbackTo("before_undelete").Error; err != nil {
		t.Fatalf("rollback to before_undelete: %v", err)
	}
}
```

- [ ] **Step 7.2: Verify the test builds**

```bash
cd services/marketplace-api
go build -tags=integration ./internal/product/...
```

Expected: no errors. Note that running the test requires `TEST_DATABASE_URL` to be set; if it is not, the `testdb.NewTx(t)` call skips the test per `pkg/testdb`'s documented behavior.

- [ ] **Step 7.3: Run the test against the compose postgres**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
# Ensure postgres and marketplace_db are up and migrated (Task 2 left the DB at version 1)
docker compose -f infra/dev/docker-compose.yml up -d postgres marketplace-api-migrate

# Run the integration test
TEST_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable \
  go -C services/marketplace-api test -tags=integration ./internal/product/... -v
```

Expected: both `TestIntegration_FullProductGraph_RoundTrip` and `TestIntegration_PartialUnique_SoftDelete` pass with `--- PASS`.

**If the test fails** with a trigger error, double-check the migration applied cleanly (`psql -d marketplace_db -c "\dS+ product_variants"` should show `inventory_quantity` as `integer`, and `\df sync_variant_inventory` should list the function).

- [ ] **Step 7.4: Commit**

The test imports `github.com/google/uuid`. It is likely already a transitive dependency (GORM, golang-migrate, and other libs commonly pull it), but verify with `git status` before committing. If `go.mod` / `go.sum` show as modified, include them in the commit explicitly; if not, omit them.

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/marketplace-api/internal/product/models_integration_test.go

# Check whether google/uuid pulled a go.mod change
if ! git diff --quiet services/marketplace-api/go.mod services/marketplace-api/go.sum; then
    git add services/marketplace-api/go.mod services/marketplace-api/go.sum
fi

git status --short
git commit -m "test(marketplace-api): round-trip integration test for product graph + triggers"
```

---

### Task 8: M2 verification + PR

This task produces no code — it verifies the M2 exit criteria and opens the PR.

- [ ] **Step 8.1: Run the full unit suite**

```bash
cd services/marketplace-api
go build ./...
go test ./...
```

Expected: all existing unit tests from M1 still pass (mode, config, health, httpserver).

- [ ] **Step 8.2: Run the integration test again to confirm reproducibility**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
TEST_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable \
  go -C services/marketplace-api test -tags=integration ./internal/product/...
```

Expected: `ok` with zero failures.

- [ ] **Step 8.3: Verify the up→down→up cycle one more time**

```bash
docker compose -f infra/dev/docker-compose.yml run --rm marketplace-api-migrate down 1
docker compose -f infra/dev/docker-compose.yml run --rm marketplace-api-migrate up
```

Expected: down reports 1 migration rolled back; up reports 1 migration applied. No errors.

- [ ] **Step 8.4: Push the branch and open a PR**

```bash
git push -u origin feat/products-m2-schema
gh pr create --base main \
  --title "feat(marketplace-api): M2 schema + domain models" \
  --body "$(cat <<'EOF'
## Summary

M2 of the products feature — lands the complete slice-1 database schema (13 tables) and the Go structs that map to it. Replaces M1's no-op scaffold migration with the real one.

See `docs/superpowers/plans/2026-04-09-products-m2-schema-and-models.md` for the task breakdown and `docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` §4 + §14 for the authoritative schema.

## What's in this PR

- **`000001_products_initial.up.sql`** — 13 tables, every constraint and index from spec §14, `sync_variant_inventory` trigger per §14.5, composite FK per §14.4, partial unique indexes per §14.3.
- **`000001_products_initial.down.sql`** — reverse-dependency drops, tested via up→down→up cycle.
- **GORM models** in 5 new internal packages: `stores/` (Store + StoreWatermark), `outbox/`, `idempotency/`, `category/`, `product/` (Product + Option + OptionValue + Variant + VariantOptionValue + Media + ProductCategory).
- **One round-trip integration test** covering the full product graph, trigger-maintained inventory sync, and partial-unique soft-delete semantics.
- **Scaffold migration deleted** — `000001_init.*` and `.keep.sql` are removed in the same commit as the real migration lands.

## What's NOT in this PR

- No repositories, no services, no HTTP handlers. M3+ handles those.
- No OpenFGA. M4 handles that.
- No outbox publisher runtime. Cemented in the schema and models here; the publisher goroutine lands in M3.

## Test plan

- [x] `go build ./...` clean
- [x] `go test ./...` passes (existing M1 unit tests unchanged)
- [x] `go test -tags=integration ./internal/product/...` passes against dev-stack postgres
- [x] `migrate up → down → up` cycle clean
- [x] 13 schema tables + 1 tracking table visible in `\dt`
- [x] `sync_variant_inventory` trigger verified by the round-trip test

## Follow-ups

- **M3** — product repositories + services (the transaction orchestration from spec §6.4 and §13.3)
- **M1.5** — deploy marketplace-api to dev cluster (still deferred)
EOF
)"
```

- [ ] **Step 8.5: Wait for CI green**

CI runs `go test ./...` on the PR via the `Go (marketplace-api)` matrix entry added in M1. Expect green because the M2 integration test is gated by `//go:build integration` and will NOT run in the default CI job — it only runs when `TEST_DATABASE_URL` is set. Unit tests from M1 keep passing.

The M3 plan will add a CI step that spins up a Postgres service container and runs the integration test against it.

---

## Exit criteria

M2 is complete when:

- [ ] Migration `000001_products_initial.{up,down}.sql` is in `main` and replaces `000001_init.*`
- [ ] All 13 tables + 1 tracking table are present after `migrate up`
- [ ] `up → down → up` cycles cleanly with zero errors
- [ ] GORM structs compile and the round-trip test passes against real Postgres
- [ ] `sync_variant_inventory` trigger is verified by the round-trip test
- [ ] Partial unique index soft-delete scenario is verified by `TestIntegration_PartialUnique_SoftDelete`
- [ ] The M2 PR is merged to main

---

## Known M2 hand-off notes for M3's plan

When M3's plan is written, it should know:

1. **No repositories yet.** M3 creates `internal/product/repository.go`, `internal/category/repository.go`, `internal/stores/repository.go`, `internal/outbox/repository.go`. They all sit on top of the models M2 shipped.
2. **Outbox publisher goroutine** is deferred from M2 to M3 — spec §14.6 calls for it in slice 1, but landing it in M2 would require wiring a background loop without any events to publish. M3's service-layer writes will produce events, and the publisher can be stood up alongside them.
3. **Store pull-through** is not exercised yet — the `stores` table exists but has zero rows in the integration test (the test inserts them directly). M3's `StoreMiddleware` per spec §14.7 lands with a mock platform-api client in tests. Real platform-api integration is M5 or later.
4. **Bluemonday sanitization policy** (spec §14.14) is an M3 concern — the `Description *string` field on Product is raw text at the DB level; the sanitizer wraps writes in `product.Service.Create/Update` which lands in M3.

## Estimated effort

| Task | Effort |
|---|---|
| 1. Migration up.sql (spec copy-paste, careful paste job) | 45 min |
| 2. Migration down.sql + up→down→up smoke | 30 min |
| 3. stores models | 10 min |
| 4. outbox + idempotency models | 20 min |
| 5. category model | 10 min |
| 6. product model (largest file) | 30 min |
| 7. round-trip test | 45 min |
| 8. verification + PR | 15 min |
| **Total** | **~3.5 hours** |

Substantially smaller than M1. No infra work, no Dockerfile, no CI changes.
