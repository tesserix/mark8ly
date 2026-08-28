-- 000110_drop_cashfree_provider.down.sql
--
-- Restores India's Cashfree option only. Deleted payment_gateway_configs rows
-- are NOT restored: they held encrypted per-store credentials that this
-- migration cannot reconstruct, and inventing empty rows would be worse than
-- their absence.
UPDATE supported_countries
   SET payment_providers = array_append(payment_providers, 'cashfree')
 WHERE country_code = 'IN'
   AND NOT ('cashfree' = ANY(payment_providers));
