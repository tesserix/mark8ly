-- 000109_email_sends_delivery_state.down.sql
DROP INDEX IF EXISTS idx_email_send_events_send_id;
DROP TABLE IF EXISTS email_send_events;
ALTER TABLE email_sends DROP COLUMN IF EXISTS last_event_id;
ALTER TABLE email_sends DROP COLUMN IF EXISTS event_at;
ALTER TABLE email_sends DROP CONSTRAINT IF EXISTS email_sends_status_valid;
ALTER TABLE email_sends ADD CONSTRAINT email_sends_status_valid
    CHECK (status IN ('sending', 'sent', 'failed'));
