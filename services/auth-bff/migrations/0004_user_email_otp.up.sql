-- Email one-time login codes. One row per issued challenge; rows are
-- kept after use so the rate limiter can still count them inside its
-- window, and purged on a timer once expires_at is well past.
--
-- code_hash is HMAC-SHA256 over (email, code) keyed by a server-side
-- pepper. Never the plaintext code.
CREATE TABLE IF NOT EXISTS user_email_otp (
    id          UUID        PRIMARY KEY,
    email       TEXT        NOT NULL,
    code_hash   BYTEA       NOT NULL,
    ip_address  TEXT        NOT NULL DEFAULT '',
    attempts    SMALLINT    NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_email_otp_attempts_sane CHECK (attempts >= 0 AND attempts <= 100)
);

-- Serves both the "newest challenge for this address" lookup and the
-- rate-limit count, which share the (email, created_at) prefix.
CREATE INDEX IF NOT EXISTS user_email_otp_email_created_idx
    ON user_email_otp (email, created_at DESC);

CREATE INDEX IF NOT EXISTS user_email_otp_expires_idx
    ON user_email_otp (expires_at);
