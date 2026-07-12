-- Reverse ONLY what 000092's up actually added: the idempotency_key column
-- and the three indexes. order_id and the status DEFAULT 'pending' both
-- predate this migration (they were created in 000008) — the up's
-- `ADD COLUMN IF NOT EXISTS order_id` / `ALTER COLUMN status SET DEFAULT`
-- were redundant no-ops against pre-existing schema, so dropping them here
-- would destroy load-bearing columns/defaults the whole refund saga relies on.
DROP INDEX IF EXISTS ix_refund_transactions_order_id;
DROP INDEX IF EXISTS ix_refund_transactions_status_created;
DROP INDEX IF EXISTS ux_refund_transactions_idempotency_key;
ALTER TABLE refund_transactions
    DROP COLUMN IF EXISTS idempotency_key;
