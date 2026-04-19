-- 000065_sea_manual_review_queue.down.sql
DROP INDEX IF EXISTS smrq_queued_week_idx;
DROP INDEX IF EXISTS smrq_country_idx;
DROP INDEX IF EXISTS smrq_status_idx;
DROP TABLE IF EXISTS sea_manual_review_queue;
