-- 000144_in_store_sales.up.sql
--
-- In-store sales (#1102): an order rung up at a counter or stall, paid
-- outside mark8ly. customer_email stays NOT NULL; a sale without a receipt
-- email stores '' and every sender skips an empty recipient.

ALTER TABLE orders
    ADD COLUMN channel varchar(20) NOT NULL DEFAULT 'online'
        CHECK (channel IN ('online', 'in_store')),
    ADD COLUMN payment_method varchar(20)
        CHECK (payment_method IN ('card_external', 'cash', 'bank_transfer', 'other')),
    ADD COLUMN payment_method_label varchar(40),
    ADD COLUMN sold_by_user_id varchar(64),
    ADD COLUMN cash_tendered numeric(12,2);

CREATE INDEX orders_store_channel_placed_idx ON orders (store_id, channel, placed_at DESC);

-- Owner switch: may cashiers give discounts and ring up custom prices.
ALTER TABLE stores
    ADD COLUMN cashier_discounts_enabled boolean NOT NULL DEFAULT false;
