-- Migration 125: journal_subscribers.unsubscribe_token — the erasure
-- mechanism promised by the customererasure declaredExclusions entry for
-- this table (internal/customererasure/coverage_integration_test.go):
-- a Journal subscriber has no store_id/tenant_id to route a store-scoped
-- erasure through, so the art.17 right is exercised directly against the
-- platform via a bearer token mailed to the subscriber, not through a
-- merchant's flow. See internal/handlers/public/journal_unsubscribe.go.
--
-- pgcrypto's gen_random_bytes is already available — migration 000058
-- ran `CREATE EXTENSION IF NOT EXISTS pgcrypto` for the exact same
-- "backfill a random secret, then drop the default" shape used there for
-- stores.storefront_customer_portal_secret. Reusing that precedent here
-- rather than generating tokens in application code for the backfill
-- keeps this migration a single, self-contained SQL file: it backfills
-- the (in practice zero, but must-be-correct-regardless) existing rows
-- in the same statement that adds the NOT NULL column, with no separate
-- Go backfill step required. Every *new* row after this migration still
-- gets its token from journal.GenerateUnsubscribeToken (crypto/rand) in
-- Go — the DEFAULT below exists purely to satisfy NOT NULL for rows that
-- predate this column, which is why it is dropped immediately after.
ALTER TABLE journal_subscribers
    ADD COLUMN unsubscribe_token CHAR(64) NOT NULL
        DEFAULT encode(gen_random_bytes(32), 'hex');

ALTER TABLE journal_subscribers ALTER COLUMN unsubscribe_token DROP DEFAULT;

-- The token is looked up by exact match on unsubscribe — a unique index
-- both enforces "one bearer credential authorises exactly one row" and
-- makes that lookup an index seek rather than a sequential scan.
CREATE UNIQUE INDEX IF NOT EXISTS idx_journal_subscribers_unsubscribe_token
    ON journal_subscribers (unsubscribe_token);
