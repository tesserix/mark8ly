-- Safe to drop: the column only ever supplies a fallback weight, and
-- checkout's own 500g default stands in when it is absent.
ALTER TABLE shipping_carrier_configs
    DROP CONSTRAINT IF EXISTS shipping_carrier_configs_default_parcel_weight_positive;

ALTER TABLE shipping_carrier_configs
    DROP COLUMN IF EXISTS default_parcel_weight_grams;
