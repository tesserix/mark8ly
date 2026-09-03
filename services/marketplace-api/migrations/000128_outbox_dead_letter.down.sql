ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS dead_lettered_at,
    DROP COLUMN IF EXISTS dead_letter_reason;
