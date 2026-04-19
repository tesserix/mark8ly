-- 000072_break_glass_accounts.up.sql
-- §12.4 One emergency local admin account per Pro+SSO tenant.
-- Plaintext password + TOTP secret live ONLY in GCP Secret Manager
-- at `secret_path`. DB holds bcrypt(password) and a JSON pointer into
-- the Secret Manager blob. Login schedules a 24h rotation; a daily
-- cron also rotates any row older than 90 days.

CREATE TABLE break_glass_accounts (
    tenant_id             UUID PRIMARY KEY,
    secret_path           TEXT NOT NULL,        -- /projects/tesserix-prod/secrets/break-glass-{uuid}
    password_hash         TEXT NOT NULL,        -- bcrypt cost 12
    totp_secret_ref       TEXT NOT NULL,        -- JSON pointer into secret blob (e.g. "$.totp_secret")
    totp_enrolled         BOOLEAN NOT NULL DEFAULT false,
    last_rotated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at          TIMESTAMPTZ,
    rotation_scheduled_at TIMESTAMPTZ,          -- set to now()+24h on successful login
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_break_glass_rotation_scheduled ON break_glass_accounts(rotation_scheduled_at)
    WHERE rotation_scheduled_at IS NOT NULL;

COMMENT ON TABLE break_glass_accounts IS 'One emergency account per Pro+SSO tenant. §12.4.';
