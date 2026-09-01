-- Restores PayPal to the countries 000090 seeded it into.
--
-- India is deliberately EXCLUDED: PayPal does not support domestic INR
-- payments for Indian merchants, so putting it back there would re-offer a
-- gateway that cannot work. If India is ever wanted, add it explicitly with
-- a reason.
--
-- Does not reactivate any payment_gateway_configs row: whether a merchant
-- wants PayPal live again is their decision, not a rollback's.
UPDATE supported_countries
   SET payment_providers = array_append(payment_providers, 'paypal')
 WHERE country_code IN (
         'US','CA','GB','DE','FR','IT','ES','NL','AU','SG','MY','TH','PH','ID'
       )
   AND NOT ('paypal' = ANY(payment_providers));
