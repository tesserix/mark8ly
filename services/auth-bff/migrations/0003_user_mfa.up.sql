-- TOTP enrollment records. One row per user. Secret is stored
-- encrypted via AES-GCM using the existing SESSION_ENCRYPT_KEY —
-- never plaintext. `enabled` flips true only after the user proves
-- possession of the secret by submitting a valid 6-digit code.
--
-- Unverified rows (enrollment started but never confirmed) sit with
-- enabled=false and are overwritten on re-enrollment.
CREATE TABLE IF NOT EXISTS user_mfa (
    user_id       TEXT        PRIMARY KEY,
    secret_enc    BYTEA       NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT FALSE,
    verified_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
