-- Initial migration. Real domain tables land in Phase C+.
-- This file exists so embed.FS has at least one match and the migrate
-- machinery has a baseline version to assert against.

CREATE TABLE IF NOT EXISTS schema_marker (
    id          SMALLINT PRIMARY KEY DEFAULT 1,
    initialized TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT schema_marker_singleton CHECK (id = 1)
);

INSERT INTO schema_marker (id) VALUES (1) ON CONFLICT DO NOTHING;
