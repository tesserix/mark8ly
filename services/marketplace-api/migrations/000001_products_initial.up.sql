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
