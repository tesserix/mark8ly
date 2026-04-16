-- Registry of active auth-bff sessions. One row per mint. Populated
-- by autologin on successful login, revoked on logout or via the
-- explicit DELETE /api/v1/sessions/:id endpoint used by the admin
-- Account Settings page.
--
-- We do NOT remove revoked rows — the revoked_at timestamp is kept
-- for a short audit history. A cron in a later phase can purge
-- rows older than e.g. 30 days.
CREATE TABLE IF NOT EXISTS user_sessions (
    id             UUID        PRIMARY KEY,
    user_id        TEXT        NOT NULL,
    tenant_id      TEXT        NOT NULL,
    device         TEXT        NOT NULL DEFAULT '',
    ip_address     TEXT        NOT NULL DEFAULT '',
    user_agent     TEXT        NOT NULL DEFAULT '',
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS user_sessions_user_active_idx
    ON user_sessions (user_id, revoked_at)
    WHERE revoked_at IS NULL;
