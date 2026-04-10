-- 000006_store_watermarks_orders.down.sql

BEGIN;

ALTER TABLE store_watermarks DROP COLUMN IF EXISTS orders_updated_at;

COMMIT;
