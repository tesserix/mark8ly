-- 000008_payments_shipping_tax: Foundation tables for payments, shipping,
-- tax, and platform fees. Seeds 15 supported countries.
BEGIN;

-- Single source of truth for which countries Mark8ly supports.
CREATE TABLE supported_countries (
    country_code  CHAR(2)      PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    currency_code CHAR(3)      NOT NULL,
    region        VARCHAR(20)  NOT NULL,
    payment_providers TEXT[]   NOT NULL,
    shipping_carriers TEXT[]   NOT NULL,
    tax_strategy  VARCHAR(20)  NOT NULL DEFAULT 'flat',
    tax_rate      NUMERIC(5,2),
    is_active     BOOLEAN      NOT NULL DEFAULT true
);

-- Seed 15 supported countries.
INSERT INTO supported_countries (country_code, name, currency_code, region, payment_providers, shipping_carriers, tax_strategy, tax_rate) VALUES
    ('IN', 'India',           'INR', 'india',    '{razorpay,paypal}',  '{delhivery}',  'india_gst', NULL),
    ('US', 'United States',   'USD', 'americas', '{stripe,paypal}',    '{shipengine}', 'taxjar',    NULL),
    ('CA', 'Canada',          'CAD', 'americas', '{stripe,paypal}',    '{shipengine}', 'flat',      5.00),
    ('GB', 'United Kingdom',  'GBP', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      20.00),
    ('DE', 'Germany',         'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      19.00),
    ('FR', 'France',          'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      20.00),
    ('IT', 'Italy',           'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      22.00),
    ('ES', 'Spain',           'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      21.00),
    ('NL', 'Netherlands',     'EUR', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      21.00),
    ('AU', 'Australia',       'AUD', 'europe',   '{stripe,paypal}',    '{shipengine}', 'flat',      10.00),
    ('SG', 'Singapore',       'SGD', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      9.00),
    ('MY', 'Malaysia',        'MYR', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      8.00),
    ('TH', 'Thailand',        'THB', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      7.00),
    ('PH', 'Philippines',     'PHP', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      12.00),
    ('ID', 'Indonesia',       'IDR', 'sea',      '{stripe,paypal}',    '{ninjavan}',   'flat',      11.00);

-- Per-store payment gateway credentials.
CREATE TABLE payment_gateway_configs (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID         NOT NULL,
    store_id             UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    provider             VARCHAR(20)  NOT NULL,
    api_key_encrypted    TEXT         NOT NULL,
    secret_key_encrypted TEXT,
    mode                 VARCHAR(10)  NOT NULL DEFAULT 'test',
    is_active            BOOLEAN      NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (store_id, provider)
);

-- Per-store shipping carrier credentials.
CREATE TABLE shipping_carrier_configs (
    id                   UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID          NOT NULL,
    store_id             UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    provider             VARCHAR(20)   NOT NULL,
    api_key_encrypted    TEXT          NOT NULL,
    secret_key_encrypted TEXT,
    mode                 VARCHAR(10)   NOT NULL DEFAULT 'test',
    is_active            BOOLEAN       NOT NULL DEFAULT false,
    warehouse_name       VARCHAR(200),
    warehouse_line1      VARCHAR(300),
    warehouse_line2      VARCHAR(300),
    warehouse_city       VARCHAR(200),
    warehouse_region     VARCHAR(200),
    warehouse_postal     VARCHAR(40),
    warehouse_country    CHAR(2),
    warehouse_phone      VARCHAR(40),
    handling_fee         NUMERIC(12,2) NOT NULL DEFAULT 0,
    free_shipping_min    NUMERIC(12,2),
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, provider)
);

-- Payment transaction records.
CREATE TABLE payment_transactions (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL,
    order_id            UUID          NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    provider            VARCHAR(20)   NOT NULL,
    provider_intent_id  VARCHAR(200),
    provider_payment_id VARCHAR(200),
    amount              NUMERIC(12,2) NOT NULL,
    currency_code       CHAR(3)       NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    payment_method      VARCHAR(40),
    metadata            JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX payment_tx_order_idx ON payment_transactions (order_id);

-- Refund transaction records.
CREATE TABLE refund_transactions (
    id                     UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              UUID          NOT NULL,
    payment_transaction_id UUID          NOT NULL REFERENCES payment_transactions(id),
    order_id               UUID          NOT NULL,
    provider_refund_id     VARCHAR(200),
    amount                 NUMERIC(12,2) NOT NULL,
    currency_code          CHAR(3)       NOT NULL,
    status                 VARCHAR(20)   NOT NULL DEFAULT 'pending',
    reason                 VARCHAR(200),
    created_at             TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- Webhook event log (idempotency guard).
CREATE TABLE webhook_events (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          VARCHAR(20)  NOT NULL,
    provider_event_id VARCHAR(200) NOT NULL,
    event_type        VARCHAR(60)  NOT NULL,
    payload           JSONB        NOT NULL,
    status            VARCHAR(20)  NOT NULL DEFAULT 'received',
    processed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_event_id)
);

-- Shipment records.
CREATE TABLE shipments (
    id                   UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID          NOT NULL,
    store_id             UUID          NOT NULL,
    order_id             UUID          NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    carrier              VARCHAR(20)   NOT NULL,
    tracking_number      VARCHAR(100),
    label_url            TEXT,
    status               VARCHAR(20)   NOT NULL DEFAULT 'pending',
    ship_from            JSONB         NOT NULL,
    ship_to              JSONB         NOT NULL,
    base_rate            NUMERIC(12,2),
    handling_fee         NUMERIC(12,2) NOT NULL DEFAULT 0,
    total_cost           NUMERIC(12,2),
    currency_code        CHAR(3)       NOT NULL,
    estimated_delivery   TIMESTAMPTZ,
    shipped_at           TIMESTAMPTZ,
    delivered_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX shipments_order_idx ON shipments (order_id);

-- Platform fee configuration (per-store).
CREATE TABLE platform_fee_configs (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID          NOT NULL,
    store_id      UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    fee_percent   NUMERIC(5,2)  NOT NULL DEFAULT 2.5,
    fee_fixed     NUMERIC(12,2) NOT NULL DEFAULT 0.30,
    fee_currency  CHAR(3)       NOT NULL DEFAULT 'USD',
    payer         VARCHAR(20)   NOT NULL DEFAULT 'merchant',
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

-- Append-only platform fee ledger.
CREATE TABLE platform_fee_ledger (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID          NOT NULL,
    store_id          UUID          NOT NULL,
    order_id          UUID          NOT NULL REFERENCES orders(id),
    transaction_type  VARCHAR(20)   NOT NULL,
    gross_amount      NUMERIC(12,2) NOT NULL,
    fee_amount        NUMERIC(12,2) NOT NULL,
    net_amount        NUMERIC(12,2) NOT NULL,
    currency_code     CHAR(3)       NOT NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX fee_ledger_order_idx ON platform_fee_ledger (order_id);
CREATE INDEX fee_ledger_store_idx ON platform_fee_ledger (store_id, created_at);

-- Per-order tax breakdown lines.
CREATE TABLE order_tax_lines (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    description   VARCHAR(100)  NOT NULL,
    rate          NUMERIC(5,2)  NOT NULL,
    amount        NUMERIC(12,2) NOT NULL,
    jurisdiction  VARCHAR(100)
);
CREATE INDEX tax_lines_order_idx ON order_tax_lines (order_id);

-- Tax provider configs (e.g. TaxJar for US stores).
CREATE TABLE tax_provider_configs (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID         NOT NULL,
    store_id          UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    provider          VARCHAR(20)  NOT NULL DEFAULT 'taxjar',
    api_key_encrypted TEXT         NOT NULL,
    mode              VARCHAR(10)  NOT NULL DEFAULT 'test',
    is_active         BOOLEAN      NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (store_id, provider)
);

-- India GST fields on products (populated only for IN stores).
ALTER TABLE products ADD COLUMN IF NOT EXISTS hsn_code VARCHAR(10);
ALTER TABLE products ADD COLUMN IF NOT EXISTS gst_rate NUMERIC(5,2);

COMMIT;
