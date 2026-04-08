-- Phase P — tenant membership invitations.
--
-- One row per "someone invited X to join tenant Y" event. The row
-- tracks pending/accepted/expired/revoked state; the actual role
-- grant lives in OpenFGA (written on accept).
--
-- token_hash stores sha256(raw_token). The raw token only ever
-- exists in the email sent to the invitee; platform-api never keeps
-- a plaintext copy. Token lookup on accept is "hash the incoming
-- token, index into this column". The hash is indexed because the
-- accept endpoint is called for every invite click.
--
-- email is lower-cased at write time and compared case-insensitively
-- on accept against the GIP token's verified email. The DB-level
-- column is a plain varchar; the app enforces the casing contract.

CREATE TABLE invitations (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email                 VARCHAR(320) NOT NULL,
    role                  VARCHAR(20) NOT NULL,
    token_hash            CHAR(64)    NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    status                VARCHAR(20) NOT NULL DEFAULT 'pending',
    invited_by_user_id    VARCHAR(128) NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    accepted_at           TIMESTAMPTZ,

    CONSTRAINT invitations_role_valid
        CHECK (role IN ('admin', 'staff', 'viewer')),
    CONSTRAINT invitations_status_valid
        CHECK (status IN ('pending', 'accepted', 'expired', 'revoked'))
);

-- Fast lookup by hashed token — this is the hot path on accept.
CREATE UNIQUE INDEX idx_invitations_token_hash ON invitations (token_hash);

-- Listing pending invitations for a tenant on the /settings/team page.
CREATE INDEX idx_invitations_tenant_status ON invitations (tenant_id, status);

-- Uniqueness: a tenant can have at most one pending invitation per
-- email at a time. Accepted/revoked rows don't block a fresh invite,
-- so the unique index is partial on status='pending'.
CREATE UNIQUE INDEX idx_invitations_tenant_email_pending
    ON invitations (tenant_id, lower(email))
    WHERE status = 'pending';
