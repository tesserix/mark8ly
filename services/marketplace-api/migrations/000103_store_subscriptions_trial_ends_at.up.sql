-- Trial end becomes storable so a platform operator can extend it (#353,
-- unblocking #286). Until now it was recomputed as created_at + 90 days at
-- seven independent sites, so there was nothing to extend.
--
-- NULL means "never extended": the effective end is
-- COALESCE(trial_ends_at, created_at + interval '90 days'). Deliberately no
-- backfill — existing rows keep deriving, and the migration cannot corrupt
-- billing data.
ALTER TABLE store_subscriptions ADD COLUMN IF NOT EXISTS trial_ends_at TIMESTAMPTZ;

-- Partial, because extensions are rare: this index stays small while giving
-- the extended branch of the trial-window queries something to use. The
-- unextended branch keeps using ss_trial_reminder_scan_idx from migration 087.
CREATE INDEX IF NOT EXISTS ss_trial_ends_at_idx
    ON store_subscriptions (trial_ends_at)
    WHERE trial_ends_at IS NOT NULL;
