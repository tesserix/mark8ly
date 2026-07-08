-- refund_transactions was already extended with order_id/idempotency_key in
-- 000092, but the payment.RefundTransaction GORM model (introduced in the
-- P2/P3/P4 payment provider work) reads/writes store_id, provider, and
-- provider_payment_id — columns that were never actually migrated in. This
-- backfills the gap so the Task 6 saga primitives (ReserveRefund /
-- FinalizeRefund) can persist rows.
--
-- payment_transaction_id and currency_code are legacy columns no longer set
-- by any code path (the model has no matching fields); relax their NOT NULL
-- constraints so inserts that don't populate them still succeed. No real
-- gateway refunds have moved money yet, so there is no data to backfill.
ALTER TABLE refund_transactions
    ADD COLUMN IF NOT EXISTS store_id            uuid,
    ADD COLUMN IF NOT EXISTS provider             varchar(20),
    ADD COLUMN IF NOT EXISTS provider_payment_id  varchar(255);

ALTER TABLE refund_transactions
    ALTER COLUMN payment_transaction_id DROP NOT NULL,
    ALTER COLUMN currency_code DROP NOT NULL;

CREATE INDEX IF NOT EXISTS ix_refund_transactions_store_id
    ON refund_transactions (store_id);
