-- Claim-first idempotency for billing mail (#381).
--
-- trial_reminders and payment_action_reminders already do this per-feature.
-- Dunning and win-back did not: both re-derive eligibility on every run
-- (dunning from audit_logs date arithmetic, win-back from an updated_at
-- window), so a second run on the same day re-sent to the same merchants.
-- That was invisible while every send was a no-op.
--
-- Generic rather than two more bespoke tables: four near-identical marker
-- tables is three too many, and this is where trial_reminders and
-- payment_action_reminders should eventually be folded in.
--
-- period_key disambiguates repeats of the same template for the same
-- subscription: the target date for dunning, the window-start date for
-- win-back. template_key alone would suppress a legitimate day-7 notice
-- after a day-5 one; period_key alone would collide across templates.
CREATE TABLE IF NOT EXISTS billing_email_sends (
    subscription_id UUID        NOT NULL,
    template_key    TEXT        NOT NULL,
    period_key      TEXT        NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, template_key, period_key)
);

-- Supports the operational question "what did we send this store, when?"
-- without scanning; the primary key already covers the claim path.
CREATE INDEX IF NOT EXISTS billing_email_sends_sent_at_idx
    ON billing_email_sends (sent_at DESC);
