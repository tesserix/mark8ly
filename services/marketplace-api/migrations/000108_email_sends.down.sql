-- 000108_email_sends.down.sql
DROP INDEX IF EXISTS idx_email_sends_stuck;
DROP INDEX IF EXISTS idx_email_sends_tenant_created_at;
DROP TABLE IF EXISTS email_sends;
