-- 000101_platform_admin_audit.up.sql
-- Platform console admin surface (#274, #275).
--
-- store_id becomes nullable because platform-originated writes (tenant
-- suspend #287, trial extend #286, purge #288) are tenant-scoped and have no
-- store. Dropping NOT NULL is a catalogue-only change in Postgres: no table
-- rewrite, no significant lock, and every existing writer keeps working.
ALTER TABLE audit_logs ALTER COLUMN store_id DROP NOT NULL;

-- Operator attribution. Dedicated columns rather than the metadata jsonb
-- because the console's console_audit_log is joined to these rows on
-- operator + timestamp, and a join predicate belongs in an indexed column.
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_operator_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS capability        TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_operator_id
    ON audit_logs (actor_operator_id)
    WHERE actor_operator_id IS NOT NULL;

-- Cross-store platform reads (#276) order by created_at across all stores.
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
    ON audit_logs (created_at DESC);

-- Replay defence for signed platform calls. The unique constraint IS the
-- check: an in-memory cache would not work, since Knative runs 0-5 replicas
-- and a replay routed to another pod would not see the original.
CREATE TABLE IF NOT EXISTS platform_request_nonces (
    nonce      UUID PRIMARY KEY,
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_platform_request_nonces_expires_at
    ON platform_request_nonces (expires_at);

-- Widen the actor_type check to admit platform console operators. A later
-- task (Task 3+) writes rows with actor_type = 'operator'; without this the
-- CHECK constraint from 000035_audit_logs rejects every such insert.
ALTER TABLE audit_logs DROP CONSTRAINT audit_logs_actor_type_chk;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_actor_type_chk
    CHECK (actor_type IN ('user', 'system', 'api', 'operator'));
