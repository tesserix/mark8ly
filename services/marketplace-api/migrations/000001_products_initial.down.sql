-- ============================================================
-- 000001 · DOWN migration — reverse-dependency drop order
-- ============================================================

BEGIN;

-- Triggers first (drops before their tables is not strictly
-- required — DROP TABLE cascades triggers — but being explicit
-- makes the rollback order match the reader's mental model)
DROP TRIGGER IF EXISTS variant_stock_sync_update  ON variant_stock;
DROP TRIGGER IF EXISTS variant_stock_sync_insert  ON variant_stock;
DROP TRIGGER IF EXISTS variants_set_updated_at    ON product_variants;
DROP TRIGGER IF EXISTS products_set_updated_at    ON products;
DROP TRIGGER IF EXISTS categories_set_updated_at  ON categories;

-- Functions after triggers
DROP FUNCTION IF EXISTS sync_variant_inventory();
DROP FUNCTION IF EXISTS set_updated_at();

-- Tables in reverse-dependency order
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS product_categories;
DROP TABLE IF EXISTS product_media;
DROP TABLE IF EXISTS variant_stock;
DROP TABLE IF EXISTS variant_option_values;
DROP TABLE IF EXISTS product_variants;         -- composite FK to products(id, store_id)
DROP TABLE IF EXISTS product_option_values;
DROP TABLE IF EXISTS product_options;
DROP TABLE IF EXISTS products;                  -- products_id_store_unique drops with the table
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS store_watermarks;
DROP TABLE IF EXISTS stores;

COMMIT;
