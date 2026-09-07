-- Restores the column. Values are NOT recoverable — the table was empty when
-- 000133 ran, and the GIP provisioning client that populated it is gone.
ALTER TABLE tenant_sso_configs ADD COLUMN IF NOT EXISTS gip_provider_id TEXT;
