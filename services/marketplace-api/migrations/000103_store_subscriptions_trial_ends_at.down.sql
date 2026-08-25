-- DESTRUCTIVE: dropping this column discards every operator-granted trial
-- extension. There is no derivation that can recover them — that is the whole
-- point of the column. Rolling back past 103 means those trials silently
-- revert to created_at + 90 days.
DROP INDEX IF EXISTS ss_trial_ends_at_idx;
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS trial_ends_at;
