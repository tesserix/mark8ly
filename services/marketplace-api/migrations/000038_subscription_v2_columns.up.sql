-- 000038_subscription_v2_columns.up.sql
-- v2.3 subscription model: add tax ID, multi-currency, PPP tier, arbitrage, app lifecycle columns.
-- All columns nullable with sensible defaults so existing rows remain valid.

ALTER TABLE store_subscriptions
    ADD COLUMN reverse_charge_tax_id     VARCHAR(50),
    ADD COLUMN tax_id_country            CHAR(2),
    ADD COLUMN tax_id_validated          BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN tax_id_validated_at       TIMESTAMPTZ,
    ADD COLUMN tax_id_name_match         VARCHAR(20)  NOT NULL DEFAULT 'not_checked'
        CHECK (tax_id_name_match IN ('matched', 'unmatched', 'not_checked')),
    ADD COLUMN billing_currency          CHAR(3),
    ADD COLUMN price_tier                VARCHAR(20)  NOT NULL DEFAULT 'developed'
        CHECK (price_tier IN ('developed', 'ppp')),
    ADD COLUMN has_white_label_app_add_on BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN arbitrage_flag            BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN app_lifecycle_status      VARCHAR(30);

-- tenant_id was NOT indexed before (per exploration); add it now for safe per-tenant scans.
CREATE INDEX IF NOT EXISTS ss_tenant_idx        ON store_subscriptions (tenant_id);
CREATE INDEX IF NOT EXISTS ss_billing_currency_idx ON store_subscriptions (billing_currency) WHERE billing_currency IS NOT NULL;
CREATE INDEX IF NOT EXISTS ss_arbitrage_idx     ON store_subscriptions (arbitrage_flag) WHERE arbitrage_flag = true;
CREATE INDEX IF NOT EXISTS ss_tax_validated_idx ON store_subscriptions (tax_id_validated, tax_id_country);
