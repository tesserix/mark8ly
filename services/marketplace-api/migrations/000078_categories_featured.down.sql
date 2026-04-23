-- 000078_categories_featured.down.sql
DROP INDEX IF EXISTS categories_featured_per_store_idx;
ALTER TABLE categories DROP COLUMN IF EXISTS featured;
