-- 000002_orders_initial.down.sql
-- Drops orders tables in reverse-dependency order.
-- set_updated_at() and all shared tables (stores, store_watermarks,
-- outbox_events, idempotency_keys) are owned by the upstream products
-- migration. Do NOT drop any of them here.

BEGIN;

DROP TABLE IF EXISTS document_number_seq;
DROP TABLE IF EXISTS abandoned_carts;
DROP TABLE IF EXISTS return_items;
DROP TABLE IF EXISTS returns;
DROP TABLE IF EXISTS order_events;
DROP TABLE IF EXISTS order_addresses;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;

COMMIT;
