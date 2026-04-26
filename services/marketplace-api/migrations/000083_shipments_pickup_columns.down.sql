-- 000083_shipments_pickup_columns.down.sql

ALTER TABLE shipments
    DROP COLUMN IF EXISTS pickup_request_id,
    DROP COLUMN IF EXISTS pickup_scheduled_for;
