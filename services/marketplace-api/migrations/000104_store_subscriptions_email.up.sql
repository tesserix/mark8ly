-- Billing mail needs somewhere to send to (#381). Until now dunning, trial
-- reminder, payment-action, win-back and trial-billed mailers passed a store
-- UUID as the "to" address, which was harmless only because every one of them
-- was wired to a no-op client.
--
-- NULL means "not known yet" and is explicitly expected: customer.updated only
-- fires on change, so rows predating this column stay NULL until
-- cmd/backfill-email reads them from Stripe. A NULL recipient is refused by
-- email.ValidateRecipient and counted as skipped — never sent, never counted
-- as delivered.
--
-- citext because email comparison is case-insensitive and we do not want two
-- rows differing only by case to read as different merchants.
CREATE EXTENSION IF NOT EXISTS citext;

ALTER TABLE store_subscriptions ADD COLUMN IF NOT EXISTS email CITEXT;
