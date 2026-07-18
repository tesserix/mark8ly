ALTER TABLE shipments
    DROP COLUMN IF EXISTS cancel_requested_at,
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS cancel_status,
    DROP COLUMN IF EXISTS cancel_action;
