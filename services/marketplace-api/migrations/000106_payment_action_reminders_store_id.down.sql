-- Reverting restores the misleading name. No data changes either way.
ALTER INDEX IF EXISTS par_store_idx RENAME TO par_subscription_idx;
ALTER TABLE payment_action_reminders RENAME COLUMN store_id TO subscription_id;
