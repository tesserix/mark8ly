-- Phase Q — separate store from tenant.
--
-- Before Phase Q a tenant WAS a store: `tenants.slug` was the
-- storefront URL and `tenants.currency_code` / `timezone` /
-- `country_code` were the store's settings. Phase Q introduces a
-- many-stores-per-tenant model so a company (tenant) can run more
-- than one storefront.
--
-- Data migration:
--   1. Create `stores` table with all the moved columns.
--   2. Backfill: one store per existing tenant, named "Main Store"
--      with the slug/currency/timezone/country values copied over.
--   3. Drop the moved columns + their constraints from `tenants`.
--
-- This is a one-shot destructive migration because we are pre-prod.
-- When prod exists, the strategy in the Phase Q planning doc calls
-- for a two-phase deploy (add stores + keep tenant columns → code
-- migrate → drop columns). For dev we run both halves together.

CREATE TABLE stores (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug            VARCHAR(63)  NOT NULL UNIQUE,
    name            VARCHAR(200) NOT NULL,
    country_code    CHAR(2)      NOT NULL,
    currency_code   CHAR(3)      NOT NULL,
    timezone        VARCHAR(64)  NOT NULL,
    logo_url        TEXT,
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT stores_status_valid
        CHECK (status IN ('active', 'suspended', 'archived')),
    CONSTRAINT stores_country_fk
        FOREIGN KEY (country_code) REFERENCES countries(code) ON DELETE RESTRICT,
    CONSTRAINT stores_currency_fk
        FOREIGN KEY (currency_code) REFERENCES currencies(code) ON DELETE RESTRICT,
    CONSTRAINT stores_timezone_fk
        FOREIGN KEY (timezone) REFERENCES timezones(id) ON DELETE RESTRICT,
    -- Same slug rules as the old tenants.slug: 3-63 lowercase
    -- alphanumeric + hyphens, no edge hyphens. Globally unique
    -- because storefront URLs must be globally unique.
    CONSTRAINT stores_slug_format
        CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$')
);

CREATE INDEX idx_stores_tenant ON stores (tenant_id);

-- Backfill: one "Main Store" per existing tenant, inheriting the
-- moved columns. Uses gen_random_uuid for the new id and copies
-- the tenant's created_at/updated_at so the store "existed" from
-- the tenant's creation timestamp.
INSERT INTO stores (
    id, tenant_id, slug, name,
    country_code, currency_code, timezone,
    status, created_at, updated_at
)
SELECT
    gen_random_uuid(),
    t.id,
    t.slug,
    'Main Store',
    t.country_code,
    t.currency_code,
    t.timezone,
    'active',
    t.created_at,
    t.updated_at
FROM tenants t;

-- Drop the moved columns and their FK constraints from tenants.
-- `tenants.slug` had the format CHECK too — dropping the column
-- drops the constraint automatically.
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_country_fk;
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_currency_fk;
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_timezone_fk;
ALTER TABLE tenants DROP COLUMN IF EXISTS slug;
ALTER TABLE tenants DROP COLUMN IF EXISTS country_code;
ALTER TABLE tenants DROP COLUMN IF EXISTS currency_code;
ALTER TABLE tenants DROP COLUMN IF EXISTS timezone;
