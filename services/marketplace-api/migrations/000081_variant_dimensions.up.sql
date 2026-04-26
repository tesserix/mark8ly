-- 000081_variant_dimensions.up.sql
-- Add per-variant package dimensions so the rate-quote pipeline can pass
-- real numbers to ShipEngine / Delhivery / etc. Without these, every
-- carrier either rejects the request (Australia Post: VAL_DIMENSION_MAX)
-- or returns inaccurate rates from an arbitrary default envelope.
--
-- Centimetres with 2 decimal places: same shape as Shopify, ShipEngine,
-- and the merchant-facing UI everywhere we've looked. Nullable so legacy
-- variants stay valid; rate-request code falls back to 30×20×10 cm
-- defaults when null.

ALTER TABLE product_variants
    ADD COLUMN length_cm NUMERIC(8, 2),
    ADD COLUMN width_cm  NUMERIC(8, 2),
    ADD COLUMN height_cm NUMERIC(8, 2);

COMMENT ON COLUMN product_variants.length_cm IS 'Package length in centimetres for shipping-rate quotes. NULL = use default.';
COMMENT ON COLUMN product_variants.width_cm  IS 'Package width in centimetres for shipping-rate quotes. NULL = use default.';
COMMENT ON COLUMN product_variants.height_cm IS 'Package height in centimetres for shipping-rate quotes. NULL = use default.';
