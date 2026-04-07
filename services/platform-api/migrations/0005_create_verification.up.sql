-- Email verification tokens for the onboarding OTP flow.
--
-- Tokens are single-use, time-bounded, and bound to both an email and an
-- onboarding session. The 6-digit code is stored as its bcrypt hash so a
-- DB dump can't be used to bypass verification.

CREATE TABLE verification_tokens (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id    UUID         NOT NULL,
    email         VARCHAR(320) NOT NULL,
    code_hash     VARCHAR(128) NOT NULL,                  -- bcrypt of the 6-digit code
    purpose       VARCHAR(40)  NOT NULL DEFAULT 'email_verification',
    attempts      SMALLINT     NOT NULL DEFAULT 0,
    max_attempts  SMALLINT     NOT NULL DEFAULT 5,
    expires_at    TIMESTAMPTZ  NOT NULL,
    consumed_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT verification_tokens_session_fk
        FOREIGN KEY (session_id) REFERENCES onboarding_sessions(id) ON DELETE CASCADE,
    CONSTRAINT verification_tokens_attempts_bounded
        CHECK (attempts >= 0 AND attempts <= max_attempts)
);

CREATE INDEX idx_verification_tokens_session ON verification_tokens (session_id);
CREATE INDEX idx_verification_tokens_email ON verification_tokens (email);
CREATE INDEX idx_verification_tokens_expires ON verification_tokens (expires_at);
