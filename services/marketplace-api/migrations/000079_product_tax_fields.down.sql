-- 000079_product_tax_fields.down.sql
ALTER TABLE products DROP COLUMN IF EXISTS tax_category;
ALTER TABLE products ALTER COLUMN tax_code TYPE VARCHAR(10);
ALTER TABLE products RENAME COLUMN tax_code TO hsn_code;
ALTER TABLE products RENAME COLUMN tax_rate_override TO gst_rate;
