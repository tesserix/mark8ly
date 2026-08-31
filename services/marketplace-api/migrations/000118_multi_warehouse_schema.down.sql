-- Reversible without loss ONLY up to PR 4: order_allocations rows are lost,
-- which is fine before PR 3 exists to write them, but from PR 4 onward
-- shipments.warehouse_id holds recorded shipment origins, and dropping it
-- here silently discards that history.
DROP INDEX IF EXISTS shipments_warehouse_idx;

DROP TABLE IF EXISTS order_allocations;

ALTER TABLE shipments  DROP COLUMN IF EXISTS warehouse_id;
ALTER TABLE warehouses DROP COLUMN IF EXISTS priority;
