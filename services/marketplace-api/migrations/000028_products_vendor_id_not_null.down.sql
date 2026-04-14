-- 000028_products_vendor_id_not_null.down.sql
ALTER TABLE products ALTER COLUMN vendor_id DROP NOT NULL;
