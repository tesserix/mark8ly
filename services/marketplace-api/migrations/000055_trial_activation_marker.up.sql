-- 000055_trial_activation_marker.up.sql
-- Idempotency marker: once stamped, the activation counter does not
-- re-fire for this store. Null means the store has not yet hit the
-- day-30 + >=1-product bar.
ALTER TABLE store_subscriptions ADD COLUMN trial_activation_marked_at TIMESTAMPTZ NULL;
