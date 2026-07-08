DROP INDEX IF EXISTS ix_refund_transactions_store_id;

ALTER TABLE refund_transactions
    ALTER COLUMN payment_transaction_id SET NOT NULL,
    ALTER COLUMN currency_code SET NOT NULL;

ALTER TABLE refund_transactions
    DROP COLUMN IF EXISTS store_id,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS provider_payment_id;
