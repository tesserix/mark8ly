-- 000039_subscription_plan_v2_rename.down.sql
ALTER TABLE store_subscriptions DROP CONSTRAINT IF EXISTS store_subscriptions_plan_check;

UPDATE store_subscriptions
SET plan = CASE plan
    WHEN 'trial'  THEN 'free'
    WHEN 'studio' THEN 'pro'  -- studio has no pre-v2.3 equivalent; collapse to pro
    ELSE plan
END
WHERE plan IN ('trial', 'studio');

ALTER TABLE store_subscriptions ALTER COLUMN plan SET DEFAULT 'free';
