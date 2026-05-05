-- 000087_subscription_has_default_payment_method.down.sql

DROP INDEX IF EXISTS ss_trial_reminder_scan_idx;
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS has_default_payment_method;
