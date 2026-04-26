-- 000083_shipments_pickup_columns.up.sql
-- Add pickup-tracking columns to shipments. These were added to the
-- ShipmentRecord GORM model (PickupRequestID + PickupScheduledFor) when
-- Delhivery auto-pickup landed, but no migration was written — so every
-- INSERT into shipments has been failing with
--   ERROR: column "pickup_request_id" of relation "shipments" does not exist
-- which means we generate real labels at the carrier (CouriersPlease,
-- Delhivery, etc.) but lose them before they hit the DB. Customer pays,
-- carrier creates the label, we 500 — the worst possible failure mode.
--
-- Both columns nullable: pre-existing rows have neither value, and only
-- carriers that support automated pickup populate them on create.

ALTER TABLE shipments
    ADD COLUMN pickup_request_id    VARCHAR(100),
    ADD COLUMN pickup_scheduled_for TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN shipments.pickup_request_id IS 'Carrier-side pickup id (Delhivery pr_id, ShipEngine pickup id, etc.). NULL for shipments without an auto-scheduled pickup.';
COMMENT ON COLUMN shipments.pickup_scheduled_for IS 'UTC timestamp when the carrier is expected to collect the parcel. Combines slot date + start. NULL when no pickup is scheduled.';
