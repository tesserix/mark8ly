-- 000090_reseed_supported_countries.up.sql
-- Re-assert the 15-country seed from migration 000008. Migration 000008's
-- INSERT was non-idempotent: if the table was ever emptied after that
-- migration applied (manual DELETE, partial DB restore, ops surgery) the
-- catalogue stays empty and the admin /settings/payments + /settings/shipping
-- pages 500 on every request via supported-providers lookup.
--
-- This migration is fully idempotent via ON CONFLICT DO NOTHING so it can
-- replay safely against any state: empty table, partial seed, or full seed.
-- Future country additions should land as their own migrations rather than
-- editing this seed in place.

INSERT INTO supported_countries (country_code, name, currency_code, region, payment_providers, shipping_carriers, tax_strategy, tax_rate) VALUES
    ('IN', 'India',           'INR', 'india',    '{razorpay,paypal}',  '{delhivery}',  'india_gst', NULL),
    ('US', 'United States',   'USD', 'americas', '{stripe,paypal}',    '{shipengine}', 'taxjar',    NULL),
    ('CA', 'Canada',          'CAD', 'americas', '{stripe,paypal}',    '{shipengine}', 'flat',      5.00),
    ('GB', 'United Kingdom',  'GBP', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      20.00),
    ('DE', 'Germany',         'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      19.00),
    ('FR', 'France',          'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      20.00),
    ('IT', 'Italy',           'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      22.00),
    ('ES', 'Spain',           'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      21.00),
    ('NL', 'Netherlands',     'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      21.00),
    ('AU', 'Australia',       'AUD', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      10.00),
    ('SG', 'Singapore',       'SGD', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      9.00),
    ('MY', 'Malaysia',        'MYR', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      8.00),
    ('TH', 'Thailand',        'THB', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      7.00),
    ('PH', 'Philippines',     'PHP', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      12.00),
    ('ID', 'Indonesia',       'IDR', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      11.00)
ON CONFLICT (country_code) DO NOTHING;
