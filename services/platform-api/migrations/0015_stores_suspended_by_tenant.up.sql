-- 0015_stores_suspended_by_tenant.up.sql
-- Records WHO suspended a store, so unsuspending a tenant does not
-- reactivate a store that was suspended individually beforehand (#287).
-- Only rows changed by a tenant-level suspension carry true.
ALTER TABLE stores
    ADD COLUMN suspended_by_tenant BOOLEAN NOT NULL DEFAULT false;

-- Partial index: the unsuspend path selects exactly these rows.
CREATE INDEX stores_suspended_by_tenant_idx
    ON stores (tenant_id)
    WHERE suspended_by_tenant;
