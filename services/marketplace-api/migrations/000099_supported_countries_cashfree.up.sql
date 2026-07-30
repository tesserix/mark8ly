-- 000099_supported_countries_cashfree.up.sql
-- Offer Cashfree in India, as the PREFERRED gateway, with Razorpay and PayPal
-- remaining selectable beneath it.
--
-- supported_countries.payment_providers is the allowlist that gates BOTH the
-- admin write path (PUT /settings/payments/:provider rejects a provider the
-- store's country does not list) and the storefront read path (the
-- payment-methods endpoint only returns configured providers that appear
-- here). Without this row change, a merchant cannot save Cashfree credentials
-- at all and the adapter is unreachable.
--
-- ORDER IS MEANINGFUL. The payment-methods endpoint sorts its response by each
-- provider's position in this array, and the storefront renders that order and
-- pre-selects the first entry. So putting 'cashfree' at the head is what makes
-- it the default choice at checkout; the others follow as radio options.
-- (Before this change the endpoint had no ORDER BY at all and the picker order
-- was whatever Postgres returned — see the sort added alongside this migration.)
--
-- Written to NORMALIZE rather than blindly prepend: array_remove strips any
-- existing occurrence before array_prepend puts it at the head, and the guard
-- skips rows already in the desired shape. That makes this idempotent on
-- replay and self-healing if an earlier run appended the value at the tail.
UPDATE supported_countries
   SET payment_providers = array_prepend('cashfree', array_remove(payment_providers, 'cashfree'))
 WHERE country_code = 'IN'
   AND payment_providers[1] IS DISTINCT FROM 'cashfree';
