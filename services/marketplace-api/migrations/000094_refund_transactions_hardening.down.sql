-- Relax the constraints added by the up migration. Safe to run even if the
-- up's guards skipped applying them (DROP ... IF EXISTS / DROP NOT NULL are
-- no-ops when the constraint was never added).
ALTER TABLE refund_transactions
    ALTER COLUMN store_id DROP NOT NULL,
    ALTER COLUMN provider DROP NOT NULL;

ALTER TABLE refund_transactions
    DROP CONSTRAINT IF EXISTS refund_transactions_status_valid;
