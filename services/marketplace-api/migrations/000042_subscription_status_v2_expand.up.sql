-- 000040_subscription_status_v2_expand.up.sql
-- Remap legacy statuses + add v2.3 states.
UPDATE store_subscriptions
SET status = CASE status
    WHEN 'cancelled'  THEN 'expired'
    WHEN 'incomplete' THEN 'signup'
    ELSE status
END
WHERE status IN ('cancelled', 'incomplete');

ALTER TABLE store_subscriptions
    ADD CONSTRAINT store_subscriptions_status_check
        CHECK (status IN (
            'signup',
            'trialing',
            'active',
            'past_due',
            'payment_action_required',
            'cancel_scheduled',
            'expired',
            'store_closed',
            'pending_hard_delete',
            'hard_deleted'
        ));
