-- Reversible without loss for the columns; order_allocations rows are lost,
-- which is correct — they only have meaning alongside the code that writes
-- them, and that code does not exist until PR 3.
DROP TABLE IF EXISTS order_allocations;

ALTER TABLE shipments  DROP COLUMN IF EXISTS warehouse_id;
ALTER TABLE warehouses DROP COLUMN IF EXISTS priority;
