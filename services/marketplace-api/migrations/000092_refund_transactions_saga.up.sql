-- Extend refund_transactions to be the saga ledger (spec §5).
-- Columns added nullable first so the migration is safe even if rows exist,
-- then tightened. No real gateway refunds have moved money yet, so backfill
-- of order_id is best-effort (left NULL for any legacy rows).
ALTER TABLE refund_transactions
    ADD COLUMN IF NOT EXISTS order_id        uuid,
    ADD COLUMN IF NOT EXISTS idempotency_key varchar(255);

-- status already referenced by the webhook handler; ensure it exists + default.
ALTER TABLE refund_transactions
    ALTER COLUMN status SET DEFAULT 'pending';

-- Backfill legacy rows to a stable synthetic key so the UNIQUE index can be
-- created without collisions.
UPDATE refund_transactions
   SET idempotency_key = 'legacy:' || id::text
 WHERE idempotency_key IS NULL;

ALTER TABLE refund_transactions
    ALTER COLUMN idempotency_key SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_refund_transactions_idempotency_key
    ON refund_transactions (idempotency_key);

CREATE INDEX IF NOT EXISTS ix_refund_transactions_status_created
    ON refund_transactions (status, created_at);

CREATE INDEX IF NOT EXISTS ix_refund_transactions_order_id
    ON refund_transactions (order_id);
