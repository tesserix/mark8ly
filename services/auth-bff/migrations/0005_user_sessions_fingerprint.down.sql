DROP INDEX IF EXISTS user_sessions_user_fingerprint_idx;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS fingerprint;
