-- 000107_inbox_action_idempotency.down.sql
ALTER TABLE migration_fast_path_reviews DROP COLUMN IF EXISTS reviewer_operator_id;
DROP INDEX IF EXISTS idx_inbox_action_idempotency_expires_at;
DROP TABLE IF EXISTS inbox_action_idempotency;
