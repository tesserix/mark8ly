-- Gift card status enum guard.
--
-- Migration 000025 intended to add this constraint but its DO $$ block
-- executes NULL; and is a no-op — the constraint has never existed in
-- production. Without it a typo'd manual UPDATE (e.g. status='disable')
-- produces a row in no known state. The redemption predicate in
-- DebitInTx requires status = 'active', so such a row is unspendable
-- (safe), but it is also unreachable by every admin filter — the worst
-- kind of silent data corruption for money-bearing rows.
--
-- DROP-then-ADD rather than IF NOT EXISTS: Postgres has no
-- ADD CONSTRAINT IF NOT EXISTS, and dropping first makes this migration
-- re-runnable.
ALTER TABLE gift_cards DROP CONSTRAINT IF EXISTS gift_cards_status_valid;

ALTER TABLE gift_cards
  ADD CONSTRAINT gift_cards_status_valid
  CHECK (status IN ('pending', 'active', 'disabled', 'depleted', 'refunded'));
