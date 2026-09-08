-- 000137_break_glass_login_attempts.up.sql
-- §12.4's sliding window, made durable (#846).
--
-- 000073 created break_glass_lockouts and its header called itself
-- "Sliding-window rate-limit + hard lockouts". Only the lockouts half was
-- ever built: the window lived in an in-memory map on one pod
-- (internal/breakglass/ratelimit.go), which is why the admin deployment was
-- pinned to a single replica. That pin is what makes every marketplace-api
-- deploy a total outage of the platform-admin surface, because the pod is
-- scaled to zero before its replacement starts (maxSurge: 0).
--
-- This table is the other half. One row per failed attempt, counted inside
-- LoginRateWindow, so the 3-strike threshold is a property of the ESTATE
-- rather than of whichever pod happened to receive the request.
--
-- ip_hash is an HMAC of the client IP under a shared key (see
-- internal/breakglass/audit.go HMACIPHash) — raw IPs are never persisted,
-- the same rule 000073 states.

CREATE TABLE break_glass_login_attempts (
    id           BIGSERIAL   PRIMARY KEY,
    ip_hash      BYTEA       NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_break_glass_login_attempts_ip_hash_attempted_at
    ON break_glass_login_attempts (ip_hash, attempted_at DESC);

COMMENT ON TABLE break_glass_login_attempts IS
    'Durable sliding window of failed /admin/break-glass/login attempts. §12.4, #846.';
