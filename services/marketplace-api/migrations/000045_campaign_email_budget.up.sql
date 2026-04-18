-- 000045_campaign_email_budget.up.sql
-- §10.1 — per-store monthly campaign email budget. Consumed atomically by pre-send enforcement (P9).

CREATE TABLE campaign_email_budget (
    store_id    UUID NOT NULL,
    month       DATE NOT NULL,
    remaining   INT  NOT NULL,
    limit_set   INT  NOT NULL,  -- mutated by trial-ramp cron D3→D4 and D7→D8
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (store_id, month)
);

-- Sanity check: can never go negative.
ALTER TABLE campaign_email_budget
    ADD CONSTRAINT campaign_email_budget_remaining_nonneg CHECK (remaining >= 0);

COMMENT ON COLUMN campaign_email_budget.limit_set IS '§5.1 — trial ramp: D1-3=500, D4-7=2000, D8+=plan allowance.';
