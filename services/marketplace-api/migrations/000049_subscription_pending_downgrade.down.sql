-- 000049_subscription_pending_downgrade.down.sql
DROP INDEX IF EXISTS ss_last_plan_change_idx;
DROP INDEX IF EXISTS ss_pending_downgrade_ready_idx;

ALTER TABLE store_subscriptions
    DROP CONSTRAINT IF EXISTS ss_pending_downgrade_consistency_check,
    DROP CONSTRAINT IF EXISTS ss_pending_downgrade_period_check,
    DROP CONSTRAINT IF EXISTS ss_pending_downgrade_plan_check,
    DROP CONSTRAINT IF EXISTS ss_subscription_period_check,
    DROP COLUMN IF EXISTS last_plan_change_reason,
    DROP COLUMN IF EXISTS last_plan_change_at,
    DROP COLUMN IF EXISTS pending_downgrade_effective_at,
    DROP COLUMN IF EXISTS pending_downgrade_period,
    DROP COLUMN IF EXISTS pending_downgrade_plan,
    DROP COLUMN IF EXISTS subscription_period;
