-- Migration 128: dead-letter columns for outbox_events (#405).
--
-- `failed` (error IS NOT NULL) is already terminal: ProcessBatch's poll
-- requires error IS NULL, so a failed row is never re-selected. What that
-- state cannot express is the difference between "the publisher gave up"
-- and "an operator decided this event will never be delivered", plus a
-- human-readable reason for the latter. dead_lettered_at is the marker for
-- that operator decision; dead_letter_reason is the reason.
--
-- A dead-lettered row is reversible: requeue clears both columns alongside
-- error. See internal/outbox/dead_letter.go.
ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS dead_lettered_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dead_letter_reason TEXT;
