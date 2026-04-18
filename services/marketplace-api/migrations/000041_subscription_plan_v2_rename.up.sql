-- 000039_subscription_plan_v2_rename.up.sql
-- Remap plan values to v2.3 lineup: trial | starter | studio | pro | marketplace.
-- Old: free | starter | pro | enterprise | marketplace.

UPDATE store_subscriptions
SET plan = CASE plan
    WHEN 'free'       THEN 'trial'
    WHEN 'enterprise' THEN 'pro'
    WHEN 'pro'        THEN 'pro'        -- legacy single-tier pro merges into new pro
    WHEN 'starter'    THEN 'starter'
    WHEN 'marketplace' THEN 'marketplace'
    ELSE plan
END
WHERE plan IN ('free', 'enterprise', 'pro', 'starter', 'marketplace');

-- Change default from 'free' to 'trial' for newly-inserted rows.
ALTER TABLE store_subscriptions
    ALTER COLUMN plan SET DEFAULT 'trial';

-- Add CHECK constraint enumerating the v2.3 values (there was none before).
ALTER TABLE store_subscriptions
    ADD CONSTRAINT store_subscriptions_plan_check
        CHECK (plan IN ('trial', 'starter', 'studio', 'pro', 'marketplace'));
