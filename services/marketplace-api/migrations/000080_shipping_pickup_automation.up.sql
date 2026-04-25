-- 000080_shipping_pickup_automation.up.sql
-- Add pickup automation columns to shipping_carrier_configs. These were
-- added to the GORM model + admin settings handler (commits referencing
-- AutoSchedulePickup/DefaultPickupSlotStart/DefaultPickupSlotEnd) without
-- a matching migration, so every save hit
--   ERROR: column "auto_schedule_pickup" of relation "shipping_carrier_configs" does not exist
-- as a 500.
--
-- Defaults mirror the GORM tags: auto_schedule_pickup TRUE, slot 14:00–18:00.
-- Existing merchants are opted into the automated pickup window the moment
-- the column lands, matching the form's default state.

ALTER TABLE shipping_carrier_configs
    ADD COLUMN auto_schedule_pickup     BOOLEAN     NOT NULL DEFAULT TRUE,
    ADD COLUMN default_pickup_slot_start VARCHAR(8) NOT NULL DEFAULT '14:00:00',
    ADD COLUMN default_pickup_slot_end   VARCHAR(8) NOT NULL DEFAULT '18:00:00';

COMMENT ON COLUMN shipping_carrier_configs.auto_schedule_pickup IS
    'Master toggle for carrier auto-scheduled pickup. When TRUE the order pipeline books a pickup in the default slot once a label is purchased.';
COMMENT ON COLUMN shipping_carrier_configs.default_pickup_slot_start IS
    'Local-time HH:MM:SS marker for the start of the merchant''s default pickup window.';
COMMENT ON COLUMN shipping_carrier_configs.default_pickup_slot_end IS
    'Local-time HH:MM:SS marker for the end of the merchant''s default pickup window. Convention: end = start + 4h.';
