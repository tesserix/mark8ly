-- 000100_razorpay_preferred_in_india.down.sql
-- Restore migration 000099's ordering: Cashfree back at the head of India's
-- payment_providers, making it the default at checkout again.
--
-- Rolling back to 99 must land in 99's state, not in 98's — so this promotes
-- Cashfree rather than removing it. Removing it here would leave the database
-- at "99 applied" while the row looks like 98, and a subsequent re-apply of
-- 000100 would then silently produce a different result.
--
-- Same remove-then-prepend normalization as the up migration, so it is
-- idempotent and independent of the current position.
UPDATE supported_countries
   SET payment_providers = array_prepend('cashfree', array_remove(payment_providers, 'cashfree'))
 WHERE country_code = 'IN';
