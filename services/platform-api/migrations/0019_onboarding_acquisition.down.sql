ALTER TABLE tenants DROP COLUMN IF EXISTS acquisition;

DROP INDEX IF EXISTS onboarding_sessions_classification_created_idx;

ALTER TABLE onboarding_sessions
    DROP CONSTRAINT IF EXISTS onboarding_sessions_classification_check;

ALTER TABLE onboarding_sessions
    DROP COLUMN IF EXISTS classification,
    DROP COLUMN IF EXISTS acquisition;
