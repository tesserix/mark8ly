-- 000067_storefront_published_flag.up.sql
-- Storefront publish gate + quarterly revalidation tracking columns (§5.3, §19.5).
-- Worker `closed.html` mechanics live in P12; here we only flip the bit
-- so cron + middleware have a place to write/read.

ALTER TABLE store_subscriptions
    ADD COLUMN storefront_published         BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN storefront_unpublished_at    TIMESTAMPTZ,
    ADD COLUMN storefront_unpublish_reason  VARCHAR(40)
        CHECK (storefront_unpublish_reason IS NULL OR storefront_unpublish_reason IN (
            'awaiting_tax_validation',
            'tax_revalidation_failed',
            'admin_action',
            'payment_terminal'
        )),
    ADD COLUMN revalidation_attempted_at    TIMESTAMPTZ,
    ADD COLUMN tax_revalidation_started_at  TIMESTAMPTZ;

-- Backfill: any active subscription with a validated tax ID is retroactively
-- considered published. Everything else stays unpublished pending validation.
UPDATE store_subscriptions
   SET storefront_published = true
 WHERE status = 'active'
   AND tax_id_validated = true;

CREATE INDEX IF NOT EXISTS ss_storefront_published_idx
    ON store_subscriptions (storefront_published) WHERE storefront_published = false;

CREATE INDEX IF NOT EXISTS ss_revalidation_due_idx
    ON store_subscriptions (revalidation_attempted_at)
 WHERE tax_id_validated = true;

CREATE INDEX IF NOT EXISTS ss_tax_revalidation_window_idx
    ON store_subscriptions (tax_revalidation_started_at)
 WHERE tax_revalidation_started_at IS NOT NULL;
