-- 000056_subscription_dunning_columns.down.sql
ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS first_charge_at,
    DROP COLUMN IF EXISTS hosted_invoice_url;
