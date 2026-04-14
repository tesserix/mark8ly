-- 000027_vendors.up.sql
-- Phase 1 of the tenant/vendor/store refactor. See
-- docs/superpowers/specs/2026-04-14-tenant-vendor-store-architecture-design.md

-- 1. vendors table
CREATE TABLE IF NOT EXISTS vendors (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(63) NOT NULL,
    status     VARCHAR(32) NOT NULL DEFAULT 'active',
    is_self    BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS vendors_slug_key ON vendors (slug);
CREATE UNIQUE INDEX IF NOT EXISTS vendors_tenant_self_idx
    ON vendors (tenant_id)
    WHERE is_self = true;
CREATE INDEX IF NOT EXISTS vendors_tenant_id_idx ON vendors (tenant_id);

-- 2. Backfill: one self-vendor per tenant with products today.
--    Name/slug are placeholders — platform-api's backfill-vendors CLI
--    overwrites them with the real tenant name + tenant-derived slug.
INSERT INTO vendors (tenant_id, name, slug, status, is_self)
SELECT DISTINCT
    p.tenant_id,
    'Merchant'                                 AS name,
    'vendor-' || REPLACE(p.tenant_id::text, '-', '') AS slug,
    'active'                                   AS status,
    true                                       AS is_self
FROM products p
WHERE p.tenant_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- 3. Backfill products.vendor_id to the tenant's self-vendor.
UPDATE products p
SET    vendor_id = v.id
FROM   vendors v
WHERE  v.tenant_id = p.tenant_id
  AND  v.is_self    = true
  AND  p.vendor_id IS NULL;
