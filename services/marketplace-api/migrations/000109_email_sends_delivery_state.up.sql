-- 000109_email_sends_delivery_state.up.sql
-- Provider delivery events for outbound mail (#348, piece B).
--
-- Piece A recorded the SEND attempt: sending -> sent | failed. That answers
-- "did we hand it to a provider", which is not the question the issue asks.
-- Provider events answer "did it arrive", so the status becomes a lifecycle:
--
--   sending -> sent -> delivered
--                   -> bounced | complained
--   sending -> failed                       (never reached a provider)
--
-- One column rather than a second `delivery_status`, because a row has one
-- furthest state and two columns would allow contradictory pairs
-- (failed + delivered) that nothing would reject.
ALTER TABLE email_sends DROP CONSTRAINT IF EXISTS email_sends_status_valid;
ALTER TABLE email_sends ADD CONSTRAINT email_sends_status_valid
    CHECK (status IN ('sending', 'sent', 'failed',
                      'delivered', 'bounced', 'complained'));

-- When the provider's event was generated, distinct from created_at (our
-- attempt) and sent_at (our hand-off). A bounce arriving hours later is
-- normal, and the gap between sent_at and this is the thing worth seeing.
ALTER TABLE email_sends ADD COLUMN IF NOT EXISTS event_at TIMESTAMPTZ;

-- Provider event id, for idempotency. Webhooks are at-least-once: a provider
-- that does not get a 2xx retries, so the SAME event arrives more than once.
-- The unique index is the check; there is no read-then-write race to lose.
ALTER TABLE email_sends ADD COLUMN IF NOT EXISTS last_event_id TEXT;

CREATE TABLE IF NOT EXISTS email_send_events (
    event_id   TEXT PRIMARY KEY,
    send_id    UUID        NOT NULL,
    event_type TEXT        NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Finding every event for one send is the debugging question.
CREATE INDEX IF NOT EXISTS idx_email_send_events_send_id
    ON email_send_events (send_id);
