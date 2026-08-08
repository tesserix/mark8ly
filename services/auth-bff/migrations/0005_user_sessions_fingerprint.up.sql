-- Device fingerprint (SHA-256 of the user agent) recorded per session
-- so a login from an unrecognised device can be detected and alerted.
--
-- Added nullable-with-default so the change is safe on a live table:
-- existing rows keep '' and are never matched, which means the first
-- login after this ships is treated as a new device for everyone. That
-- is the intended, one-off behaviour.
ALTER TABLE user_sessions
    ADD COLUMN IF NOT EXISTS fingerprint TEXT NOT NULL DEFAULT '';

-- Supports the "has this account used this device before" existence
-- check, which runs once per login.
CREATE INDEX IF NOT EXISTS user_sessions_user_fingerprint_idx
    ON user_sessions (user_id, fingerprint)
    WHERE fingerprint <> '';
