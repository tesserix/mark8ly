-- 000068_enterprise_api_keys.down.sql
DROP INDEX IF EXISTS eak_store_active_idx;
DROP INDEX IF EXISTS eak_tenant_prefix_uniq;
DROP TABLE IF EXISTS enterprise_api_keys;
