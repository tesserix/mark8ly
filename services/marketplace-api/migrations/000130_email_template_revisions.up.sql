-- 000130_email_template_revisions.up.sql
-- Operator attribution for authored email templates (tesserix-home#588).
--
-- Every other write on the platform admin surface records itself in
-- audit_logs. An email template cannot: audit_logs is tenant partitioned
-- (tenant_id NOT NULL, migration 000035 — 000101 relaxed store_id for
-- tenant-scoped platform writes and deliberately left tenant_id alone),
-- and email_templates.key is estate-wide. There is no tenant to scope the
-- row to and inventing one would put a fiction in a governance table.
--
-- So the record lives here, written on the SAME transaction as the change
-- it accounts for (internal/emailtemplates.Store.Upsert): a failed record
-- rolls the change back rather than leaving a template edit unattributed.
--
-- Structural facts only, never the authored copy. The live email_templates
-- row already holds the copy, and a trail that duplicates the data it
-- exists to account for is a second copy of the problem with a longer
-- retention. This is consequently NOT a version history and offers no
-- rollback — tesserix-home#588 rules that out explicitly.
CREATE TABLE IF NOT EXISTS email_template_revisions (
    id         bigserial   PRIMARY KEY,
    key        text        NOT NULL,
    -- The version the row HAS after the change, matching
    -- email_templates.version. Not a foreign key: a template row is
    -- upserted in place and the trail must outlive any future delete of it.
    version    integer     NOT NULL,
    status     text        NOT NULL CHECK (status IN ('draft','published')),
    -- The signed platform operator id, never a value from the request body.
    changed_by text        NOT NULL,
    -- The capability the console asserted. Nullable: this surface records
    -- the capability it was given and does not yet require a value
    -- (platformadmin.CapabilityValueChecked is false).
    capability text,
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_template_revisions_key_changed_at
    ON email_template_revisions (key, changed_at DESC);
