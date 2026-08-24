DROP INDEX IF EXISTS stores_suspended_by_tenant_idx;
ALTER TABLE stores DROP COLUMN IF EXISTS suspended_by_tenant;
