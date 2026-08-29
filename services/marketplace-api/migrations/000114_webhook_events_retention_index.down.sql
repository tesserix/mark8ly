-- 000114 down — drop the webhook_events retention prune index (#440).
-- The prune cron still runs correctly without it; it just seq-scans.
DROP INDEX IF EXISTS idx_webhook_events_status_created_at;
