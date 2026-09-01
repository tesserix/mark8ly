-- Stop offering PayPal until it has been validated against a real account.
--
-- `paypal.go` is a genuine 483-line gateway with unit tests, and it is wired
-- into payment.NewGateway — so a merchant in any supported country can open
-- Settings → Payments, click "Add credentials" on PayPal, and put a gateway
-- nobody has ever run against live PayPal in front of their customers.
--
-- The admin card list and the storefront's payment methods both read this
-- column (see handlers/admin/settings_meta.go and
-- handlers/storefront/payment_methods.go — "making that array the single
-- source"), so removing the entry hides PayPal everywhere without touching
-- a line of the implementation. Re-adding it is a one-line migration once
-- someone has taken a real PayPal payment end to end.
--
-- India additionally must NOT get PayPal back by default even after that:
-- PayPal discontinued DOMESTIC payments for Indian merchants in 2021 and
-- supports only cross-border, so an INR store selling to Indian customers
-- cannot use it. 000100 already demoted PayPal there; this removes it.
UPDATE supported_countries
   SET payment_providers = array_remove(payment_providers, 'paypal')
 WHERE 'paypal' = ANY(payment_providers);

-- Any store that somehow configured PayPal is deactivated rather than
-- deleted: the credentials stay encrypted at rest so re-enabling is a flip,
-- not a re-entry. Expected to affect zero rows today.
UPDATE payment_gateway_configs
   SET is_active = false, updated_at = now()
 WHERE provider = 'paypal' AND is_active = true;
