-- 000082_orders_shipping_choice.down.sql

ALTER TABLE orders
    DROP COLUMN IF EXISTS shipping_service,
    DROP COLUMN IF EXISTS shipping_carrier;
