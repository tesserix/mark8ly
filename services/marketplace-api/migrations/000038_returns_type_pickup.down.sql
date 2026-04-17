DROP INDEX IF EXISTS returns_store_status_requested_idx;

ALTER TABLE returns DROP CONSTRAINT IF EXISTS returns_type_valid;

ALTER TABLE returns
    DROP COLUMN IF EXISTS reject_reason,
    DROP COLUMN IF EXISTS rejected_at,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS pickup_details,
    DROP COLUMN IF EXISTS type;
