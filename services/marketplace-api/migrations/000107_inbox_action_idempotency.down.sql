-- 000107_inbox_action_idempotency.down.sql
DROP INDEX IF EXISTS idx_inbox_action_idempotency_expires_at;
DROP TABLE IF EXISTS inbox_action_idempotency;
