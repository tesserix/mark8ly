-- Initial migration for auth-bff. Real session/MFA tables land in Phase E.

CREATE TABLE IF NOT EXISTS schema_marker (
    id          SMALLINT PRIMARY KEY DEFAULT 1,
    initialized TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT schema_marker_singleton CHECK (id = 1)
);

INSERT INTO schema_marker (id) VALUES (1) ON CONFLICT DO NOTHING;
