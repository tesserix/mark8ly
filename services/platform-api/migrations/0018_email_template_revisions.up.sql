-- email_template_revisions — operator attribution for authored templates
-- served on the platform admin contract surface (mark8ly#720 Task 5).
--
-- Mirrors marketplace-api's migration 000130 field for field, for the same
-- reason: this service has no tenant-scoped audit table to record an
-- email-template edit against. platform-api's audit trail is
-- internal/audit.Client, which posts to marketplace-api's
-- /internal/audit-events ingest and REQUIRES a non-empty tenant_id
-- (Client.Emit no-ops with a warning otherwise) — and an email template
-- key is estate-wide, so there is no tenant to supply. Inventing one would
-- put a fiction in a governance table, so the record lives here instead,
-- written on the SAME transaction as the change it accounts for
-- (internal/emailtemplates.Store.Upsert): a failed insert here rolls the
-- change back rather than leaving an edit unattributed.
--
-- Structural facts only, never the authored copy — see marketplace-api's
-- 000130 for the fuller rationale, which applies unchanged here.
CREATE TABLE IF NOT EXISTS email_template_revisions (
    id         bigserial   PRIMARY KEY,
    key        text        NOT NULL,
    version    integer     NOT NULL,
    status     text        NOT NULL CHECK (status IN ('draft','published')),
    changed_by text        NOT NULL,
    capability text,
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_template_revisions_key_changed_at
    ON email_template_revisions (key, changed_at DESC);
