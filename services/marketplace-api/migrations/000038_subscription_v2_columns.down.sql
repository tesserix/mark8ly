-- 000038_subscription_v2_columns.down.sql
DROP INDEX IF EXISTS ss_tax_validated_idx;
DROP INDEX IF EXISTS ss_arbitrage_idx;
DROP INDEX IF EXISTS ss_billing_currency_idx;
DROP INDEX IF EXISTS ss_tenant_idx;

ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS app_lifecycle_status,
    DROP COLUMN IF EXISTS arbitrage_flag,
    DROP COLUMN IF EXISTS has_white_label_app_add_on,
    DROP COLUMN IF EXISTS price_tier,
    DROP COLUMN IF EXISTS billing_currency,
    DROP COLUMN IF EXISTS tax_id_name_match,
    DROP COLUMN IF EXISTS tax_id_validated_at,
    DROP COLUMN IF EXISTS tax_id_validated,
    DROP COLUMN IF EXISTS tax_id_country,
    DROP COLUMN IF EXISTS reverse_charge_tax_id;
