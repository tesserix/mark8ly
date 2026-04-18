-- 000049_subscription_pending_downgrade.up.sql
-- v2.3 §4.5 + §4.5.1: pending-downgrade fields + subscription_period column.
-- Columns nullable where appropriate; consistency CHECK enforces the pending trio.

ALTER TABLE store_subscriptions
    ADD COLUMN subscription_period            VARCHAR(10)  NOT NULL DEFAULT 'monthly',
    ADD COLUMN pending_downgrade_plan         VARCHAR(30),
    ADD COLUMN pending_downgrade_period       VARCHAR(10),
    ADD COLUMN pending_downgrade_effective_at TIMESTAMPTZ,
    ADD COLUMN last_plan_change_at            TIMESTAMPTZ,
    ADD COLUMN last_plan_change_reason        VARCHAR(64),
    ADD CONSTRAINT ss_subscription_period_check CHECK (
        subscription_period IN ('monthly','annual')
    ),
    ADD CONSTRAINT ss_pending_downgrade_plan_check CHECK (
        pending_downgrade_plan IS NULL
        OR pending_downgrade_plan IN ('trial','starter','studio','pro','marketplace')
    ),
    ADD CONSTRAINT ss_pending_downgrade_period_check CHECK (
        pending_downgrade_period IS NULL
        OR pending_downgrade_period IN ('monthly','annual')
    ),
    ADD CONSTRAINT ss_pending_downgrade_consistency_check CHECK (
        (pending_downgrade_plan IS NULL AND pending_downgrade_period IS NULL AND pending_downgrade_effective_at IS NULL)
        OR (pending_downgrade_plan IS NOT NULL AND pending_downgrade_period IS NOT NULL AND pending_downgrade_effective_at IS NOT NULL)
    );

-- Cron reads this partial index exclusively.
CREATE INDEX IF NOT EXISTS ss_pending_downgrade_ready_idx
    ON store_subscriptions (pending_downgrade_effective_at)
    WHERE pending_downgrade_effective_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS ss_last_plan_change_idx
    ON store_subscriptions (last_plan_change_at);
