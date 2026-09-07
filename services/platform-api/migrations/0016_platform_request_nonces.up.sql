-- 0016_platform_request_nonces.up.sql
-- Replay defence for the signed platform console surface (#720 Task 3),
-- ported from marketplace-api's own copy of this table
-- (marketplace-api/migrations/000101_platform_admin_audit.up.sql). Not a
-- shared table: platform-api and marketplace-api have separate databases,
-- so each service owns and claims nonces against its own copy.
--
-- The unique constraint on nonce (the primary key) IS the replay check —
-- platformauth.NonceStore.Claim relies on INSERT ... ON CONFLICT DO NOTHING
-- rather than a read-then-write race. An in-memory cache would not work
-- here either: this service runs multiple replicas, and a replay routed to
-- a different pod would not see the original claim.
CREATE TABLE IF NOT EXISTS platform_request_nonces (
    nonce      UUID PRIMARY KEY,
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_platform_request_nonces_expires_at
    ON platform_request_nonces (expires_at);
