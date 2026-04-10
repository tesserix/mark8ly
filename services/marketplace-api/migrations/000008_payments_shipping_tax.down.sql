-- 000008_payments_shipping_tax (down): reverse in dependency order.
BEGIN;

ALTER TABLE products DROP COLUMN IF EXISTS gst_rate;
ALTER TABLE products DROP COLUMN IF EXISTS hsn_code;

DROP TABLE IF EXISTS tax_provider_configs;
DROP TABLE IF EXISTS order_tax_lines;
DROP TABLE IF EXISTS platform_fee_ledger;
DROP TABLE IF EXISTS platform_fee_configs;
DROP TABLE IF EXISTS shipments;
DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS refund_transactions;
DROP TABLE IF EXISTS payment_transactions;
DROP TABLE IF EXISTS shipping_carrier_configs;
DROP TABLE IF EXISTS payment_gateway_configs;
DROP TABLE IF EXISTS supported_countries;

COMMIT;
