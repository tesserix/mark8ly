-- 000113 — GDPR art.17 erasure EXECUTION (#259).
--
-- Two things, both prerequisites for a worker that can actually run.
--
-- 1. A state machine. The original CHECK admitted only
--    ('pending','processed','rejected') — there was no in-flight state at
--    all, so a worker could not mark a row as being worked on, and a crash
--    mid-erasure was indistinguishable from one never started. #259
--    describes 'processing'/'completed'/'failed' as already existing; they
--    did not. 'processed' is kept as a terminal alias so existing rows stay
--    valid and no backfill is needed.
--
--    Constraint name verified against the live schema before this migration
--    was written: customer_erasure_requests_status_check.
ALTER TABLE customer_erasure_requests
    DROP CONSTRAINT IF EXISTS customer_erasure_requests_status_check;

ALTER TABLE customer_erasure_requests
    ADD CONSTRAINT customer_erasure_requests_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'processed', 'rejected'));

-- attempts bounds retry: a request that fails repeatedly must stop being
-- retried forever and become visible to an operator instead.
ALTER TABLE customer_erasure_requests
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;

-- 2. The retention basis, written down where it is enforced.
--    Until now the ONLY retention text in any migration was
--    billing_archive's (000046). The erasure below ANONYMISES rather than
--    deletes these tables, and that choice needs its justification recorded
--    next to the data, not only in a design document.
COMMENT ON TABLE orders IS
    'Financial record. Personal fields are anonymised on GDPR art.17 erasure; the row is retained 7 years under legal-obligation basis, matching billing_archive (§23.2). See #259.';
COMMENT ON TABLE order_addresses IS
    'Financial record (delivery evidence). Personal fields anonymised on erasure; country_code retained for tax reporting. 7-year legal-obligation retention (§23.2). See #259.';
COMMENT ON TABLE payment_transactions IS
    'Financial record. Anonymised, not deleted, on GDPR art.17 erasure; retained 7 years under legal-obligation basis (§23.2). See #259.';
COMMENT ON TABLE refund_transactions IS
    'Financial record. Anonymised, not deleted, on GDPR art.17 erasure; retained 7 years under legal-obligation basis (§23.2). See #259.';
COMMENT ON TABLE platform_fee_ledger IS
    'Financial record. Anonymised, not deleted, on GDPR art.17 erasure; retained 7 years under legal-obligation basis (§23.2). See #259.';
