-- #484 (step 1 of the contract half of #177): close the backfill gap that
-- blocked dropping the legacy warehouse_* columns on
-- shipping_carrier_configs.
--
-- 000095's backfill only ran once, at migration time. Any
-- shipping_carrier_configs row created (or edited) AFTER 000095 ran but
-- BEFORE #480's write path shipped still has its legacy warehouse_*
-- columns populated and warehouse_id NULL — #480's write path is the only
-- thing that ever sets warehouse_id, and it only runs on a save through
-- the admin settings handler. Every reader in this same PR stopped
-- consulting the legacy columns, so a row like that would silently lose
-- its pickup address unless this migration catches it first.
--
-- This mirrors 000095_warehouses.up.sql's backfill shape exactly —
-- including the ON CONFLICT (store_id, name) DO NOTHING and skipping rows
-- with a blank warehouse_name (never a usable warehouse, and the thing
-- Delhivery keys on) — but is scoped to today's stragglers rather than the
-- one-time full-table backfill 000095 already did. Idempotent: re-running
-- it inserts nothing new and updates nothing once every row is caught up.
INSERT INTO warehouses (
    tenant_id, store_id, name, line1, line2, city, region, postal_code,
    country_code, phone, is_default
)
SELECT DISTINCT ON (c.store_id, c.warehouse_name)
    c.tenant_id,
    c.store_id,
    c.warehouse_name,
    coalesce(c.warehouse_line1, ''),
    coalesce(c.warehouse_line2, ''),
    coalesce(c.warehouse_city, ''),
    coalesce(c.warehouse_region, ''),
    coalesce(c.warehouse_postal, ''),
    coalesce(c.warehouse_country, ''),
    coalesce(c.warehouse_phone, ''),
    true
FROM shipping_carrier_configs c
WHERE coalesce(c.warehouse_name, '') <> ''
  AND c.warehouse_id IS NULL
ORDER BY c.store_id, c.warehouse_name, c.created_at
ON CONFLICT (store_id, name) DO NOTHING;

UPDATE shipping_carrier_configs c
SET warehouse_id = w.id
FROM warehouses w
WHERE w.store_id = c.store_id
  AND w.name = c.warehouse_name
  AND c.warehouse_id IS NULL;
