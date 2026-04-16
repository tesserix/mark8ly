-- User profile rows for the admin Account Settings page. One row per
-- GIP user. user_id is the GIP sub (string, not UUID). Persisted fields:
-- display_name, phone, avatar_url. Email is mirrored from the session
-- header on upsert so the row can be queried standalone.
CREATE TABLE IF NOT EXISTS user_profiles (
    user_id       TEXT        PRIMARY KEY,
    email         TEXT        NOT NULL DEFAULT '',
    display_name  TEXT        NOT NULL DEFAULT '',
    phone         TEXT        NOT NULL DEFAULT '',
    avatar_url    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_profiles_email_idx ON user_profiles (email);
