-- Migration 129: disable/enable columns for break_glass_accounts (#404).
--
-- disabled_at marks an emergency account as administratively disabled by a
-- platform operator via POST /admin/break-glass/:tenantId/disable — distinct
-- from a normal wrong-password failure or an IP lockout. The login handler
-- (internal/handlers/admin/break_glass_login.go) refuses a disabled account
-- with a response BYTE-IDENTICAL to a wrong-password failure
-- (401 {"error":"invalid_credentials"}); disabled_reason is for forensics in
-- the audit log only, and must never reach the HTTP response.
--
-- Enable (POST .../enable) clears both columns. Rotate (POST .../rotate)
-- deliberately does NOT touch either column — see internal/breakglass/
-- rotation.go's ReplaceAfterRotation, which this migration does not change.
ALTER TABLE break_glass_accounts
    ADD COLUMN IF NOT EXISTS disabled_at     TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS disabled_reason TEXT;
