-- 000135 — retire geo-pricing arbitrage detection (mark8ly#704).
--
-- arbitrage_flag could never be set to true by any code path: the only
-- caller of the recorder that would flip it hard-codes IPCountry: "" on the
-- Stripe webhook (radar has no IP), and the evaluator never flags without an
-- IP country. The table, the flag column, the cron, the appeal flow and the
-- admin/inbox surfaces built around that condition are removed together.

DROP INDEX IF EXISTS ss_arbitrage_idx;

ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS arbitrage_flag;

DROP TABLE IF EXISTS subscription_arbitrage_audit;
