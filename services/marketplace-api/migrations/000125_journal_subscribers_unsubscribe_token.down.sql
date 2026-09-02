DROP INDEX IF EXISTS idx_journal_subscribers_unsubscribe_token;
ALTER TABLE journal_subscribers DROP COLUMN IF EXISTS unsubscribe_token;
