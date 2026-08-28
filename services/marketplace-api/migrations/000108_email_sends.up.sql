-- 000108_email_sends.up.sql
-- Outbound email send log (#348, piece A).
--
-- Transactional mail was fire-and-forget and unrecorded: every mailer handed
-- an envelope to email.Sender and nothing wrote a row, so "did the merchant
-- get the email?" was unanswerable from our own data.
--
-- The id IS the correlation key. It is minted per send and injected into the
-- provider's custom_args (SendGrid echoes them on every engagement event;
-- Resend mirrors them as tags), so piece B can attribute a provider event to
-- a row without a join table or a second identifier to keep in step.
CREATE TABLE IF NOT EXISTS email_sends (
    id         UUID PRIMARY KEY,
    -- Nullable: platform-level mail (signup, anomaly cron) has no tenant or
    -- store. A NOT NULL here would force those sends to invent one.
    tenant_id  UUID,
    store_id   UUID,
    -- Required. "Did the merchant get it" is unanswerable without it.
    recipient  TEXT        NOT NULL,
    -- Structured, lowercase snake_case; 'unknown' when a mailer supplies no
    -- attribution, so an unattributed sender shows up as queryable rather
    -- than as an empty string nobody notices.
    kind       VARCHAR(64) NOT NULL,
    status     VARCHAR(16) NOT NULL,
    error      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ,
    CONSTRAINT email_sends_status_valid
        CHECK (status IN ('sending', 'sent', 'failed'))
);

-- No `subject` and no rendered body, deliberately. Subject lines are
-- interpolated customer content ("Your order #1234 from Acme Ltd"), and three
-- prior platform-console endpoints excluded exactly that: message (#332),
-- description (#329), payload (#331). A cross-tenant send log would be the
-- first to carry it. `kind` answers "which email was this" and is strictly
-- more queryable than free text.
--
-- No `provider` either: FallbackSender tries SendGrid then Resend, and the
-- decorator wraps the WHOLE chain, observing one Send and one error. It
-- cannot know which provider accepted the mail. Recording the configured
-- primary would lie in exactly the case that matters — a fallback during an
-- outage. Piece B supplies it, since provider events identify themselves.

-- The platform read (piece D) orders by time within a tenant.
CREATE INDEX IF NOT EXISTS idx_email_sends_tenant_created_at
    ON email_sends (tenant_id, created_at DESC);

-- Partial: stuck rows are the ones worth finding, and this keeps the index
-- tiny on a shared db-f1-micro instead of indexing every send ever made.
CREATE INDEX IF NOT EXISTS idx_email_sends_stuck
    ON email_sends (created_at)
    WHERE status = 'sending';
