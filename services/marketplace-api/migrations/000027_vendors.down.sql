-- 000027_vendors.down.sql
-- Reverses the vendors table creation. Intentionally does NOT try to
-- preserve products.vendor_id backfill — Phase 1 is fully reversible.

DROP INDEX IF EXISTS vendors_tenant_id_idx;
DROP INDEX IF EXISTS vendors_tenant_self_idx;
DROP INDEX IF EXISTS vendors_slug_key;
DROP TABLE IF EXISTS vendors;

-- NOTE: products.vendor_id is left populated. Harmless because the
-- column is still nullable at this point. Migration 000028 NOT NULL
-- is reversed by its own down.sql before this one runs.
