-- 000101_platform_admin_audit.down.sql
--
-- Restore the three-value actor_type check. This fails if any operator
-- rows exist (actor_type = 'operator') — that is deliberate, matching the
-- store_id rollback below: this migration will NOT delete audit rows to
-- make itself possible. Losing integrity records to a rollback is worse
-- than a rollback that stops and asks. Resolve the rows by hand, then
-- re-run.
--
-- Wrapped in a transaction so a partial failure (e.g. the ADD CONSTRAINT
-- below rejecting existing operator rows) rolls back the whole script
-- instead of leaving audit_logs with the DROP CONSTRAINT committed and no
-- actor_type validation at all. Run this with a tool that honours BEGIN/
-- COMMIT as one transaction (golang-migrate, or `psql -1 -f`) — a plain
-- `psql -f` runs unwrapped DDL in separate implicit transactions and would
-- otherwise commit the DROP before the ADD fails.
BEGIN;

ALTER TABLE audit_logs DROP CONSTRAINT audit_logs_actor_type_chk;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_actor_type_chk
    CHECK (actor_type IN ('user', 'system', 'api'));

-- Restoring NOT NULL fails if any platform-written row exists. That is
-- deliberate: this migration will NOT delete audit rows to make itself
-- possible. Losing integrity records to a rollback is worse than a rollback
-- that stops and asks. Resolve the rows by hand, then re-run.
DROP TABLE IF EXISTS platform_request_nonces;

DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_actor_operator_id;

ALTER TABLE audit_logs DROP COLUMN IF EXISTS capability;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_operator_id;

ALTER TABLE audit_logs ALTER COLUMN store_id SET NOT NULL;

COMMIT;
