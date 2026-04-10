BEGIN;
DROP INDEX IF EXISTS orders_store_email_idx;
DROP TABLE IF EXISTS customer_addresses;
DROP TABLE IF EXISTS customer_profiles;
COMMIT;
