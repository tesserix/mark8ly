-- Harden refund_transactions now that the saga moves real money.
--
-- 1. status must be one of the three lifecycle states the coordinator writes.
--    The retry sweeper re-drives 'pending' rows; a permanently-failed refund
--    is moved to 'failed' so it stops being retried forever.
-- 2. store_id / provider are required by the saga (and the payment.RefundTransaction
--    GORM model tags them NOT NULL); enforce that at the DB level so a future
--    insert path that forgets them can't silently write a tenant-orphaned row
--    that is invisible to WHERE store_id = ? filters.
--
-- Every tightening is guarded so the migration is safe even if legacy rows
-- exist: each constraint is applied only when the current data already
-- satisfies it (the saga authors confirmed no real refunds have moved money
-- yet, so in practice the table is empty). If a guard skips because of legacy
-- NULLs/values, a follow-up cleanup + re-apply is needed — but the deploy
-- never fails.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM refund_transactions WHERE status NOT IN ('pending','succeeded','failed')
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'refund_transactions_status_valid'
    ) THEN
        ALTER TABLE refund_transactions
            ADD CONSTRAINT refund_transactions_status_valid
            CHECK (status IN ('pending','succeeded','failed'));
    END IF;

    IF NOT EXISTS (SELECT 1 FROM refund_transactions WHERE store_id IS NULL) THEN
        ALTER TABLE refund_transactions ALTER COLUMN store_id SET NOT NULL;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM refund_transactions WHERE provider IS NULL) THEN
        ALTER TABLE refund_transactions ALTER COLUMN provider SET NOT NULL;
    END IF;
END $$;
