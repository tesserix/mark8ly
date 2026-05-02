-- email_templates — runtime-editable templates with embedded fallback.
--
-- Authored from tesserix-home (super-admin) via the cross-DB grant on
-- mark8ly_platform_admin (see tesserix-k8s/docs/cross-db-admin.md). Read
-- by the notification package on every send via a per-process TTL cache.
-- If a row is missing or invalid the package falls back to the embedded
-- version compiled into the binary so emails keep flowing.
--
-- Subject is itself a Go template — some emails (welcome, invitation)
-- interpolate the tenant name into the subject line.
--
-- variables is the declared variable schema (an array of {name, type})
-- so the admin UI knows which placeholders the operator can use.
CREATE TABLE email_templates (
    key          text         PRIMARY KEY,
    subject      text         NOT NULL,
    html_body    text         NOT NULL,
    text_body    text         NOT NULL,
    variables    jsonb        NOT NULL DEFAULT '[]'::jsonb,
    status       text         NOT NULL DEFAULT 'published'
                                  CHECK (status IN ('draft','published')),
    version      integer      NOT NULL DEFAULT 1,
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    updated_by   text
);

-- Default-privileges (set once via the cross-db-admin runbook) auto-grants
-- SELECT/INSERT/UPDATE/DELETE to mark8ly_platform_admin for any new table
-- in the public schema, so tesserix-home can write here without a
-- per-table GRANT statement here.
