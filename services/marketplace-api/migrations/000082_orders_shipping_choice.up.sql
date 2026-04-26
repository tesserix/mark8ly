-- 000082_orders_shipping_choice.up.sql
-- Persist the carrier + service the customer picked at checkout, so the
-- admin "Approve & generate label" panel can default to the right values
-- (an AU store running ShipEngine should not default to Delhivery).
--
-- Both columns nullable: pre-existing orders never had this captured and
-- the admin form falls back to the merchant's first configured carrier
-- when these are empty.

ALTER TABLE orders
    ADD COLUMN shipping_service VARCHAR(40),
    ADD COLUMN shipping_carrier VARCHAR(40);

COMMENT ON COLUMN orders.shipping_service IS 'Service level the customer picked at checkout (e.g. standard, express). NULL for orders pre-dating this column.';
COMMENT ON COLUMN orders.shipping_carrier IS 'Resolved carrier provider name at checkout (e.g. shipengine, delhivery, ninjavan). NULL for orders pre-dating this column.';
