-- 000051_migration_fast_path_reviews.up.sql
-- CSM review queue for merchants migrating from other platforms (Shopify, WooCommerce, BigCommerce).
-- Approval shortens the tax-ID window from 14d to 48h (see migration 053) but does NOT waive validation.
CREATE TABLE migration_fast_path_reviews (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL,
    evidence_type   TEXT         NOT NULL CHECK (evidence_type IN ('whois_domain', 'platform_screenshot')),
    evidence_url    TEXT         NOT NULL,
    prior_platform  TEXT         NULL,
    whois_domain    TEXT         NULL,
    status          TEXT         NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewer_id     UUID         NULL,
    reviewer_notes  TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    reviewed_at     TIMESTAMPTZ  NULL,
    CONSTRAINT only_one_open_per_store
        EXCLUDE (store_id WITH =) WHERE (status = 'pending')
);
CREATE INDEX idx_mfpr_pending ON migration_fast_path_reviews (status, created_at) WHERE status = 'pending';
CREATE INDEX idx_mfpr_store   ON migration_fast_path_reviews (store_id);
