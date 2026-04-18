-- 000057_payment_action_reminders.up.sql
-- Idempotency bookkeeping for §4.7 payment_action_required reminders.
-- Composite PK on (subscription_id, offset_key) means a second cron tick for
-- the same offset is a no-op via INSERT ... ON CONFLICT DO NOTHING.
CREATE TABLE payment_action_reminders (
    subscription_id  UUID         NOT NULL,
    offset_key       TEXT         NOT NULL CHECK (offset_key IN ('t_minus_14', 't_minus_7', 't_minus_1')),
    sent_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, offset_key)
);
CREATE INDEX par_subscription_idx ON payment_action_reminders (subscription_id);
