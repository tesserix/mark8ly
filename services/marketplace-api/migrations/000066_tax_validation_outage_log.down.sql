-- 000066_tax_validation_outage_log.down.sql
DROP INDEX IF EXISTS tvol_registry_idx;
DROP INDEX IF EXISTS tvol_store_idx;
DROP INDEX IF EXISTS tvol_open_idx;
DROP TABLE IF EXISTS tax_validation_outage_log;
