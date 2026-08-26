-- payment_action_reminders.subscription_id has always held a STORE id
-- SendPaymentActionReminders inserts row.StoreID while the structurally
-- identical trial_reminders table holds a real subscription id. Nothing
-- misbehaves today because store and subscription are 1:1, but migration 105
-- records the intent to fold both tables into billing_email_sends, and that
-- migration would silently move store ids into a subscription_id column.
--
-- Renaming is behaviourally inert: existing rows keep their values, which were
-- store ids all along. Deliberately NOT changing what the code inserts --
-- that would strand every existing claim and re-send reminders merchants
-- have already had.
ALTER TABLE payment_action_reminders RENAME COLUMN subscription_id TO store_id;
ALTER INDEX IF EXISTS par_subscription_idx RENAME TO par_store_idx;
