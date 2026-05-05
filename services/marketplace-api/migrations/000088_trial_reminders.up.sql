-- 000088_trial_reminders.up.sql
-- Idempotency bookkeeping for the trial-end reminder cron.
--
-- Two cadences run from the same table, distinguished by the offset_key prefix:
--   no_pm_t_minus_15 / _10 / _7 / _3 / _1  → merchants without a default
--                                            payment method get nudged five
--                                            times during the final 15 days.
--   has_pm_t_minus_1                       → merchants with a card on file
--                                            get one heads-up the day before
--                                            Stripe auto-bills the chosen plan.
--
-- Composite PK on (subscription_id, offset_key) means a second cron tick for
-- the same offset is a no-op via INSERT ... ON CONFLICT DO NOTHING. Multiple
-- replicas of the cron pod can race safely — only the first inserter sends.
--
-- tenant_id and store_id are denormalised onto each reminder row for two
-- reasons: (1) cheap multi-tenant filtering in audits ("show me reminders
-- for this tenant"), and (2) referential clarity if subscription_id is
-- ever soft-deleted.
CREATE TABLE trial_reminders (
    subscription_id  UUID         NOT NULL,
    tenant_id        UUID         NOT NULL,
    store_id         UUID         NOT NULL,
    offset_key       TEXT         NOT NULL CHECK (offset_key IN (
        'no_pm_t_minus_15',
        'no_pm_t_minus_10',
        'no_pm_t_minus_7',
        'no_pm_t_minus_3',
        'no_pm_t_minus_1',
        'has_pm_t_minus_1'
    )),
    sent_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, offset_key)
);

CREATE INDEX tr_tenant_idx       ON trial_reminders (tenant_id);
CREATE INDEX tr_subscription_idx ON trial_reminders (subscription_id);
