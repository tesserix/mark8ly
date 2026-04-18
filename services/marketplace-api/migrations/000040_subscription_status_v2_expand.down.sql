-- 000040_subscription_status_v2_expand.down.sql
ALTER TABLE store_subscriptions DROP CONSTRAINT IF EXISTS store_subscriptions_status_check;

-- Collapse v2.3-only statuses back onto the legacy set.
UPDATE store_subscriptions
SET status = CASE status
    WHEN 'signup'                  THEN 'incomplete'
    WHEN 'payment_action_required' THEN 'past_due'
    WHEN 'cancel_scheduled'        THEN 'active'
    WHEN 'store_closed'            THEN 'cancelled'
    WHEN 'pending_hard_delete'     THEN 'cancelled'
    WHEN 'hard_deleted'            THEN 'cancelled'
    WHEN 'expired'                 THEN 'cancelled'
    ELSE status
END;
