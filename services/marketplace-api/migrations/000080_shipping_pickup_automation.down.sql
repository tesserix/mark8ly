-- 000080_shipping_pickup_automation.down.sql

ALTER TABLE shipping_carrier_configs
    DROP COLUMN IF EXISTS auto_schedule_pickup,
    DROP COLUMN IF EXISTS default_pickup_slot_start,
    DROP COLUMN IF EXISTS default_pickup_slot_end;
