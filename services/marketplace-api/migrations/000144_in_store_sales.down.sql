-- 000144_in_store_sales.down.sql
ALTER TABLE stores DROP COLUMN IF EXISTS cashier_discounts_enabled;
DROP INDEX IF EXISTS orders_store_channel_placed_idx;
ALTER TABLE orders
    DROP COLUMN IF EXISTS cash_tendered,
    DROP COLUMN IF EXISTS sold_by_user_id,
    DROP COLUMN IF EXISTS payment_method_label,
    DROP COLUMN IF EXISTS payment_method,
    DROP COLUMN IF EXISTS channel;
