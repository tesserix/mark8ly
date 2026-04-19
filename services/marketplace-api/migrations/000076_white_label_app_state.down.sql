-- 000076_white_label_app_state.down.sql
-- Rollback drops the mutable state table. The append-only lifecycle log
-- (migration 000048) is untouched; historical transitions remain queryable.

DROP TABLE IF EXISTS white_label_app_state;
