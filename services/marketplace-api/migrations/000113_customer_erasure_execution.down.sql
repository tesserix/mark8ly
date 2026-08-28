ALTER TABLE customer_erasure_requests DROP COLUMN IF EXISTS attempts;

-- Rows in a state the old constraint forbids must be normalised before it
-- can be re-added, otherwise the ALTER fails on real data.
UPDATE customer_erasure_requests SET status = 'processed' WHERE status = 'completed';
UPDATE customer_erasure_requests SET status = 'pending'   WHERE status IN ('processing', 'failed');

ALTER TABLE customer_erasure_requests
    DROP CONSTRAINT IF EXISTS customer_erasure_requests_status_check;
ALTER TABLE customer_erasure_requests
    ADD CONSTRAINT customer_erasure_requests_status_check
    CHECK (status IN ('pending', 'processed', 'rejected'));

COMMENT ON TABLE orders IS NULL;
COMMENT ON TABLE order_addresses IS NULL;
COMMENT ON TABLE payment_transactions IS NULL;
COMMENT ON TABLE refund_transactions IS NULL;
COMMENT ON TABLE platform_fee_ledger IS NULL;
