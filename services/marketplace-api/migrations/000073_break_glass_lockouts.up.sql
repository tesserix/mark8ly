-- 000073_break_glass_lockouts.up.sql
-- §12.4 Sliding-window rate-limit + hard lockouts. ip_hash is an HMAC
-- of the client IP under a shared key (see internal/breakglass/audit.go
-- HMACIPHash) — raw IPs are never persisted.

CREATE TABLE break_glass_lockouts (
    ip_hash      BYTEA NOT NULL,         -- HMAC-SHA256 of client IP
    tenant_id    UUID,
    locked_until TIMESTAMPTZ NOT NULL,
    reason       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ip_hash, locked_until)
);

-- Note: a WHERE predicate using now() is illegal here (partial-index
-- predicates must be IMMUTABLE). A full index on locked_until is fine —
-- the table is small (at most a few rows per locked IP) and lookups
-- always filter by locked_until>now() in the query layer.
CREATE INDEX idx_break_glass_lockouts_locked_until
    ON break_glass_lockouts(locked_until);

COMMENT ON TABLE break_glass_lockouts IS '3-strike rate-limit lockouts for /admin/break-glass/login. §12.4.';
