-- 000110_drop_cashfree_provider.up.sql
-- Remove Cashfree as a selectable payment provider.
--
-- The adapter, its resolver branch, its webhook signature case and its
-- client-side confirm endpoint are all gone. This strips it from the DATA so
-- the two cannot disagree.
--
-- Why this migration is not optional. India's seed still read
-- `razorpay,paypal,cashfree` (verified in production). Removing the code
-- alone would leave a provider advertised at checkout that the resolver can
-- no longer construct — `unsupported payment provider: cashfree` — so an
-- Indian buyer selecting it would hit a failure the storefront still offered
-- them. The migration runs in the init container before the new server
-- accepts traffic, so the option disappears before the code that would refuse
-- it starts serving.
--
-- Written to NORMALIZE rather than assume a shape, following 000100: strip
-- the entry wherever it appears rather than rewriting the whole array, so a
-- store whose list was reordered by hand is not clobbered.
UPDATE supported_countries
   SET payment_providers = array_remove(payment_providers, 'cashfree')
 WHERE 'cashfree' = ANY(payment_providers);

-- Per-store gateway credentials. Empty in production at time of writing, but
-- a store that had configured Cashfree would otherwise keep encrypted keys
-- for an adapter that no longer exists.
DELETE FROM payment_gateway_configs WHERE provider = 'cashfree';
