-- A merchant's typical parcel weight, used when a product carries none.
--
-- Storefront checkout hardcoded `500` grams for any item without a weight
-- (apps/storefront/app/checkout/page.tsx). Shoppers pay carrier rates
-- derived from that number, so an invisible constant in frontend code was
-- setting real prices — and a store selling 200g tanks or 3kg cookware had
-- no way to correct it.
--
-- Defaults to 500 precisely so this migration changes NO live quote: every
-- existing config keeps the value checkout was already assuming. It only
-- makes the number visible and adjustable.
--
-- NOT a replacement for per-product weights, which remain the accurate
-- answer. This is the fallback when one is missing.
ALTER TABLE shipping_carrier_configs
    ADD COLUMN IF NOT EXISTS default_parcel_weight_grams integer NOT NULL DEFAULT 500;

ALTER TABLE shipping_carrier_configs
    ADD CONSTRAINT shipping_carrier_configs_default_parcel_weight_positive
    CHECK (default_parcel_weight_grams > 0);
