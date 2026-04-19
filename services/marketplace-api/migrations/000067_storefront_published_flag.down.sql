-- 000067_storefront_published_flag.down.sql
DROP INDEX IF EXISTS ss_tax_revalidation_window_idx;
DROP INDEX IF EXISTS ss_revalidation_due_idx;
DROP INDEX IF EXISTS ss_storefront_published_idx;

ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS tax_revalidation_started_at,
    DROP COLUMN IF EXISTS revalidation_attempted_at,
    DROP COLUMN IF EXISTS storefront_unpublish_reason,
    DROP COLUMN IF EXISTS storefront_unpublished_at,
    DROP COLUMN IF EXISTS storefront_published;
