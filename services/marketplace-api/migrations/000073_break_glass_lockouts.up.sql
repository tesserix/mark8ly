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

CREATE INDEX idx_break_glass_lockouts_active ON break_glass_lockouts(locked_until)
    WHERE locked_until > now();

COMMENT ON TABLE break_glass_lockouts IS '3-strike rate-limit lockouts for /admin/break-glass/login. §12.4.';
