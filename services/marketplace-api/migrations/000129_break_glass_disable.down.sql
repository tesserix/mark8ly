ALTER TABLE break_glass_accounts
    DROP COLUMN IF EXISTS disabled_at,
    DROP COLUMN IF EXISTS disabled_reason;
