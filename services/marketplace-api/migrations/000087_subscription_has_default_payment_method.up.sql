-- 000087_subscription_has_default_payment_method.up.sql
-- Track whether the Stripe customer attached to a store subscription has a
-- default payment method set. The flag is the source of truth for the trial
-- reminder cron's cadence selection (no-PM = nudges at T-15/10/7/3/1; has-PM
-- = single T-1 heads-up before auto-billing).
--
-- The column is updated reactively from Stripe webhooks: customer.updated is
-- authoritative (payload carries invoice_settings.default_payment_method).
-- payment_method.attached / .detached are kept as no-ops because Stripe also
-- emits customer.updated whenever the default changes.
--
-- A backfill is shipped separately (scripts/backfill_has_default_payment_method.go)
-- because populating from Stripe requires API calls per row.

ALTER TABLE store_subscriptions
    ADD COLUMN has_default_payment_method BOOLEAN NOT NULL DEFAULT false;

-- Partial index supports the trial reminder cron's hot path:
-- "find trialing/signup subs whose trial expires in N days and have_PM=true|false".
-- Filtering at the index level keeps the cron query small even as the table grows.
CREATE INDEX IF NOT EXISTS ss_trial_reminder_scan_idx
    ON store_subscriptions (status, created_at)
    WHERE status IN ('signup', 'trialing');
