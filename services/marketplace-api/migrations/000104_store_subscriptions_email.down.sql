-- DESTRUCTIVE: drops every merchant billing address mirrored from Stripe.
-- Recoverable by re-running cmd/backfill-email, since Stripe remains the
-- source of truth — but every cron reverts to sending nothing in the
-- meantime, because a NULL recipient is refused.
--
-- The citext extension is deliberately NOT dropped: other objects may depend
-- on it, and dropping a shared extension during a rollback is how you take
-- down unrelated tables.
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS email;
