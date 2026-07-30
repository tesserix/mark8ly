-- 000099_supported_countries_cashfree.down.sql
-- Remove Cashfree from India's payment provider allowlist.
--
-- Deliberately does NOT delete any payment_gateway_configs row a merchant
-- saved for Cashfree. Dropping the allowlist entry is enough to stop new
-- checkouts choosing it (the storefront filters its provider list through this
-- array), and destroying stored credentials on a rollback would mean a
-- re-apply requires every merchant to re-enter their keys. Orphaned rows are
-- inert and are cleaned up by the merchant removing the provider in admin.
UPDATE supported_countries
   SET payment_providers = array_remove(payment_providers, 'cashfree')
 WHERE country_code = 'IN';
