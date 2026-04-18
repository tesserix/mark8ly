-- 000052_trial_banner_state.down.sql
ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS trial_banner_state_set_at,
    DROP COLUMN IF EXISTS trial_banner_state;
