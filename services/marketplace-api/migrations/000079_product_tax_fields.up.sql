-- 000079_product_tax_fields.up.sql
-- Generalize the India-specific tax columns on `products` so every
-- supported country's tax strategy (india_gst, flat_rate, taxjar) can
-- read the same classification fields. The old names stay data-
-- compatible — `hsn_code` becomes `tax_code` (widened to 32 chars so
-- it also holds TaxJar TIC codes / EU CN codes / AU tax category
-- strings) and `gst_rate` becomes `tax_rate_override`.
--
-- A new `tax_category` column surfaces the
-- standard / reduced / zero_rated / exempt selector used by flat-rate
-- countries (GB/DE/FR/IT/ES/NL/AU/CA/SG/MY/TH/PH/ID). Existing rows
-- default to NULL → the application treats that as "standard", so the
-- migration is non-breaking.

ALTER TABLE products
    RENAME COLUMN hsn_code TO tax_code;

ALTER TABLE products
    ALTER COLUMN tax_code TYPE VARCHAR(32);

ALTER TABLE products
    RENAME COLUMN gst_rate TO tax_rate_override;

ALTER TABLE products
    ADD COLUMN tax_category VARCHAR(32);

-- No data backfill: existing India-store products keep whatever HSN /
-- GST rate they had (now visible under the renamed columns). Other
-- stores had NULLs; they stay NULL and the tax service falls back to
-- the country-default tax_rate on supported_countries.

COMMENT ON COLUMN products.tax_code IS
    'Strategy-specific classification string: HSN for IN, TaxJar TIC for US, optional tax category code for EU/UK/AU flat-rate countries.';
COMMENT ON COLUMN products.tax_rate_override IS
    'Per-product tax rate as a percentage (e.g. 18.00 = 18%). When non-null this wins over the country default; always wins for IN.';
COMMENT ON COLUMN products.tax_category IS
    'High-level classification: standard / reduced / zero_rated / exempt. NULL = standard. Used by flat-rate countries to pick the right rate tier.';
