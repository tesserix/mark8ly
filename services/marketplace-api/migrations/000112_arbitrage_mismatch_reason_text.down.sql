-- Irreversible in general: rows written after the widening may exceed 100
-- chars, and narrowing would fail or truncate. Truncate explicitly so the
-- down migration is deterministic rather than erroring on real data.
UPDATE subscription_arbitrage_audit
   SET mismatch_reason = left(mismatch_reason, 100)
 WHERE mismatch_reason IS NOT NULL
   AND length(mismatch_reason) > 100;

ALTER TABLE subscription_arbitrage_audit
    ALTER COLUMN mismatch_reason TYPE VARCHAR(100);
