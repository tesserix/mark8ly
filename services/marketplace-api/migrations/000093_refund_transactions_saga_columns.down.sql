DROP INDEX IF EXISTS ix_refund_transactions_store_id;

-- Intentionally NOT restoring NOT NULL on payment_transaction_id /
-- currency_code: the relaxation in the up migration is deliberately
-- one-way. Those columns are legacy/orphaned (no code path sets them) and
-- the saga primitives (ReserveRefund/FinalizeRefund) insert real rows that
-- never populate them. Restoring the constraint here would make `migrate
-- down` fail with a not-null violation on any environment where the saga
-- has already run, leaving golang-migrate in a dirty state.
ALTER TABLE refund_transactions
    DROP COLUMN IF EXISTS store_id,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS provider_payment_id;
