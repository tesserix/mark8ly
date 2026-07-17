-- Reverse 000095. Safe: the warehouse_* columns on shipping_carrier_configs
-- were never dropped by the up migration and remain the populated source of
-- truth, so dropping warehouses loses nothing.

DROP INDEX IF EXISTS shipping_carrier_configs_warehouse_idx;

ALTER TABLE shipping_carrier_configs
    DROP COLUMN IF EXISTS warehouse_id;

DROP TABLE IF EXISTS warehouses;
