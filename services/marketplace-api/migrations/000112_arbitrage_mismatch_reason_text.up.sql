-- 000112 — widen subscription_arbitrage_audit.mismatch_reason to TEXT (#398).
--
-- The column is used as an append-only narrative: the evaluator's reason,
-- then one "MERCHANT_APPEAL ..." block per appeal. That is unbounded by
-- design, so varchar(100) could not hold the code's own output — a
-- dual-signal flag (68-char reason) plus the 36-char appeal boilerplate is
-- 104 chars before a single character of merchant text, and the UPDATE
-- failed with SQLSTATE 22001.
--
-- TEXT rather than a wider varchar: there is no defensible finite bound on
-- "reason plus N appeals", and Postgres stores both identically.
ALTER TABLE subscription_arbitrage_audit
    ALTER COLUMN mismatch_reason TYPE TEXT;

COMMENT ON COLUMN subscription_arbitrage_audit.mismatch_reason IS
    'Append-only narrative: evaluator reason, then one MERCHANT_APPEAL block per appeal. TEXT because it is unbounded (#398). Truncate at display sites, not on write.';
