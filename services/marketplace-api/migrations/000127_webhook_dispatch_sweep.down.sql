ALTER TABLE webhook_dispatch_cursor
    DROP COLUMN IF EXISTS swept_created,
    DROP COLUMN IF EXISTS swept_id;

-- Restore 000126's tenant-only partial index.
DROP INDEX IF EXISTS idx_webhook_subs_tenant_store_enabled;

CREATE INDEX IF NOT EXISTS idx_webhook_subs_tenant_enabled
    ON webhook_subscriptions (tenant_id) WHERE enabled;
