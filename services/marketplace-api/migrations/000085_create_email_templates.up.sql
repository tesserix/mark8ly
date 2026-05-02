-- email_templates — runtime-editable transactional email templates with
-- embedded fallback. Same shape as platform-api's table (mark8ly
-- platform-api migration 0013) so the tesserix-home admin UI treats
-- them uniformly.
--
-- Authored from tesserix-home over the cross-DB grant on
-- mark8ly_platform_admin (see tesserix-k8s/docs/cross-db-admin.md).
-- Read by internal/emailtemplates.Loader on every send through a
-- 5-minute TTL cache. Missing rows / DB errors fall back to the
-- embedded version compiled into the binary so emails keep flowing.
--
-- Subject is itself a Go template — orderdoc emails interpolate the
-- order number / refund flag into the subject line. Heading + lede +
-- CTA copy are now inlined into the body templates (using {{if}} blocks)
-- rather than computed in Go, so an operator editing the template can
-- change the wording without a code change.
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
