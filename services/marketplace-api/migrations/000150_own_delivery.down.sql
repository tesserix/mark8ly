DROP INDEX IF EXISTS shipments_one_active_own_per_order;
ALTER TABLE shipments
    DROP COLUMN IF EXISTS tracking_url,
    DROP COLUMN IF EXISTS courier_name,
    DROP COLUMN IF EXISTS courier_id,
    ALTER COLUMN carrier TYPE varchar(20);
ALTER TABLE orders
    DROP COLUMN IF EXISTS shipping_option_estimate,
    DROP COLUMN IF EXISTS shipping_option_name;
DROP TABLE IF EXISTS store_couriers;
DROP TABLE IF EXISTS store_delivery_options;
ALTER TABLE stores
    DROP COLUMN IF EXISTS own_free_shipping_min,
    DROP COLUMN IF EXISTS shipping_mode;
