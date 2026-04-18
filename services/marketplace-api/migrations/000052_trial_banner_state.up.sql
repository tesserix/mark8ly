-- 000052_trial_banner_state.up.sql
-- Trial banner state doubles as the banner cron's idempotency marker (§17.5).
-- Each row is touched at most once per day; the state flip excludes it from subsequent cron predicates.
ALTER TABLE store_subscriptions
    ADD COLUMN trial_banner_state        TEXT        NULL
        CHECK (trial_banner_state IN ('none', 'day_60', 'day_75', 'day_85')),
    ADD COLUMN trial_banner_state_set_at TIMESTAMPTZ NULL;
