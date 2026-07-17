-- Extract the warehouse into a first-class, store-level entity.
--
-- Until now a "warehouse" was 8 warehouse_* columns hanging off
-- shipping_carrier_configs. That row is per (store, carrier), so a merchant
-- running Delhivery AND CouriersPlease had to type the same physical address
-- twice, into two rows, and keep them in sync by hand. The address is a
-- property of the STORE, not of the carrier account used to ship from it.
--
-- This is the expand half of an expand/contract migration:
--   * create warehouses
--   * backfill one row per distinct (store_id, warehouse_name)
--   * add shipping_carrier_configs.warehouse_id and point it at the new row
--   * LEAVE the old warehouse_* columns in place and still populated
--
-- Nothing is dropped here on purpose: the running code still reads the old
-- columns, so this migration must be a no-op for it. The contract half (drop
-- the columns) only lands once every reader goes through warehouse_id.
--
-- Multi-warehouse itself is out of scope — see #177. This just stops the
-- duplication and gives that work something to build on.

CREATE TABLE IF NOT EXISTS warehouses (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid         NOT NULL,
    store_id    uuid         NOT NULL,
    name        varchar(200) NOT NULL,
    line1       varchar(300) NOT NULL DEFAULT '',
    line2       varchar(300) NOT NULL DEFAULT '',
    city        varchar(200) NOT NULL DEFAULT '',
    region      varchar(200) NOT NULL DEFAULT '',
    postal_code varchar(40)  NOT NULL DEFAULT '',
    country_code char(2)     NOT NULL DEFAULT '',
    phone       varchar(40)  NOT NULL DEFAULT '',
    -- Carriers that validate a pickup contact (Delhivery's clientwarehouse
    -- create takes contact_person + email) currently have no column to read
    -- from: shipments.go falls back to the BUYER's order email for pickup_email,
    -- which is wrong and was flagged in a TODO there. Nullable for now — the
    -- admin UI does not collect them yet.
    email          varchar(200),
    contact_person varchar(200),
    -- First warehouse for a store is its default. Multi-warehouse (#177) will
    -- use this to pick an origin until a real allocation rule exists.
    is_default  boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- A store cannot have two pickup locations with the same name: the name is the
-- key Delhivery matches on (pickup_location.name), so duplicates would be
-- ambiguous at the carrier.
CREATE UNIQUE INDEX IF NOT EXISTS warehouses_store_name_key
    ON warehouses (store_id, name);

CREATE INDEX IF NOT EXISTS warehouses_store_idx ON warehouses (store_id);
CREATE INDEX IF NOT EXISTS warehouses_tenant_idx ON warehouses (tenant_id);

ALTER TABLE shipping_carrier_configs
    ADD COLUMN IF NOT EXISTS warehouse_id uuid REFERENCES warehouses (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS shipping_carrier_configs_warehouse_idx
    ON shipping_carrier_configs (warehouse_id);

-- Backfill. Guarded so re-running is a no-op, and skipped entirely for rows
-- with no warehouse configured (name is what Delhivery keys on — a blank name
-- was never a usable warehouse).
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
ORDER BY c.store_id, c.warehouse_name, c.created_at
ON CONFLICT (store_id, name) DO NOTHING;

UPDATE shipping_carrier_configs c
SET warehouse_id = w.id
FROM warehouses w
WHERE w.store_id = c.store_id
  AND w.name = c.warehouse_name
  AND c.warehouse_id IS NULL;
