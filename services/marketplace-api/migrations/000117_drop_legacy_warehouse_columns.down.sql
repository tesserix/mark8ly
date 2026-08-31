-- Re-open the expand half: put the columns back, with the same types
-- 000008 gave them, and re-populate them from the warehouses row each
-- config points at.
--
-- This restores every row that has a warehouse_id — which, after 000116's
-- backfill, is every row that ever had a usable warehouse. Rows left with
-- a NULL warehouse_id are the ones whose warehouse_name was blank: 000095
-- and 000116 both skipped them on purpose (a blank name was never a usable
-- warehouse, it is the field the carrier keys on), so there is nothing to
-- restore for them and they come back NULL.
--
-- Reverting the schema does NOT revert the code: a marketplace-api built
-- after #486 neither reads nor writes these columns, so they will sit
-- frozen at whatever this migration puts in them.

ALTER TABLE shipping_carrier_configs
    ADD COLUMN IF NOT EXISTS warehouse_name    VARCHAR(200),
    ADD COLUMN IF NOT EXISTS warehouse_line1   VARCHAR(300),
    ADD COLUMN IF NOT EXISTS warehouse_line2   VARCHAR(300),
    ADD COLUMN IF NOT EXISTS warehouse_city    VARCHAR(200),
    ADD COLUMN IF NOT EXISTS warehouse_region  VARCHAR(200),
    ADD COLUMN IF NOT EXISTS warehouse_postal  VARCHAR(40),
    ADD COLUMN IF NOT EXISTS warehouse_country CHAR(2),
    ADD COLUMN IF NOT EXISTS warehouse_phone   VARCHAR(40);

UPDATE shipping_carrier_configs c
   SET warehouse_name    = w.name,
       warehouse_line1   = w.line1,
       warehouse_line2   = w.line2,
       warehouse_city    = w.city,
       warehouse_region  = w.region,
       warehouse_postal  = w.postal_code,
       warehouse_country = w.country_code,
       warehouse_phone   = w.phone
  FROM warehouses w
 WHERE w.id = c.warehouse_id;
