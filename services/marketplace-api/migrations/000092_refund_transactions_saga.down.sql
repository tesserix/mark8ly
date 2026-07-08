DROP INDEX IF EXISTS ix_refund_transactions_order_id;
DROP INDEX IF EXISTS ix_refund_transactions_status_created;
DROP INDEX IF EXISTS ux_refund_transactions_idempotency_key;
ALTER TABLE refund_transactions ALTER COLUMN status DROP DEFAULT;
ALTER TABLE refund_transactions
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS order_id;
