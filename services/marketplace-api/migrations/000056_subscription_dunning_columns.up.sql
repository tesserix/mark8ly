-- 000056_subscription_dunning_columns.up.sql
-- P6 adds two columns to store_subscriptions:
--   hosted_invoice_url: stored when Stripe fires invoice.payment_action_required,
--                       surfaced by /subscription/complete-action redirect (§4.7).
--   first_charge_at:    timestamp of the first successful invoice.paid. Drives the
--                       §16.5 refund-window guard (14d window from first charge).
-- Both NULL by default — trial subs have neither until they complete checkout.
ALTER TABLE store_subscriptions
    ADD COLUMN hosted_invoice_url TEXT        NULL,
    ADD COLUMN first_charge_at    TIMESTAMPTZ NULL;
