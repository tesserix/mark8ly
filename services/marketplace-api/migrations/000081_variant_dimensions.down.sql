-- 000081_variant_dimensions.down.sql

ALTER TABLE product_variants
    DROP COLUMN IF EXISTS length_cm,
    DROP COLUMN IF EXISTS width_cm,
    DROP COLUMN IF EXISTS height_cm;
