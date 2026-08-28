-- 000111 — §5.1 trial ramp idempotency (#399).
-- ramp_step_applied records the highest ramp transition day already applied to
-- this row (0 = none, 4 = D3->D4 applied, 8 = D7->D8 applied). ApplyTrialRamp
-- guards its UPDATE on it, so a re-run within the same UTC day (multi-pod
-- scheduling, retry, replay, backfill) cannot re-apply the GREATEST ceiling and
-- refund budget the merchant has already consumed.
--
-- DEFAULT 0 backfills existing rows as "no ramp applied". That is deliberately
-- permissive: a store mid-trial gets one more ramp application, matching
-- today's behaviour exactly once, and never again.
ALTER TABLE campaign_email_budget
    ADD COLUMN IF NOT EXISTS ramp_step_applied SMALLINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN campaign_email_budget.ramp_step_applied IS
    '§5.1 — highest trial-ramp transition day applied (0/4/8). Guards ApplyTrialRamp against re-inflating consumed budget (#399).';
