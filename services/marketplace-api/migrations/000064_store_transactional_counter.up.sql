-- 000064_store_transactional_counter.up.sql
-- Tracks transactional (non-campaign) email volume per store-month.
-- Separate table — NEVER join to campaign_email_budget. Transactional sends
-- bypass the campaign budget entirely; this table only powers the 100k/store/month
-- fair-use soft cap and an ops dashboard.

CREATE TABLE IF NOT EXISTS store_transactional_counter (
    store_id  UUID NOT NULL,
    month     DATE NOT NULL,
    sent      INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (store_id, month)
);

CREATE INDEX IF NOT EXISTS stc_month_idx ON store_transactional_counter (month);
