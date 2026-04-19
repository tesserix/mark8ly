-- 000065_sea_manual_review_queue.up.sql
-- SEA (MY, TH, PH, ID, VN) tax-ID manual review. Any ID that enters this queue
-- immediately pauses the 14-day validation clock on the associated subscription
-- (§5.2). 5-biz-day SLA; sustained >30/week over 2 weeks triggers capacity alert.

CREATE TABLE IF NOT EXISTS sea_manual_review_queue (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID        NOT NULL,
    store_id           UUID        NOT NULL,
    country            CHAR(2)     NOT NULL
        CHECK (country IN ('MY', 'TH', 'PH', 'ID', 'VN')),
    tax_id             VARCHAR(50) NOT NULL,
    business_name      TEXT,
    queue_reason       VARCHAR(50) NOT NULL,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_review', 'approved', 'rejected')),
    reviewer_id        UUID,
    reviewer_notes     TEXT,
    sla_due_at         TIMESTAMPTZ NOT NULL,
    queued_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at        TIMESTAMPTZ,

    UNIQUE (tenant_id, store_id, country)
);

CREATE INDEX IF NOT EXISTS smrq_status_idx       ON sea_manual_review_queue (status) WHERE status IN ('pending', 'in_review');
CREATE INDEX IF NOT EXISTS smrq_country_idx      ON sea_manual_review_queue (country);
CREATE INDEX IF NOT EXISTS smrq_queued_week_idx  ON sea_manual_review_queue (queued_at);
