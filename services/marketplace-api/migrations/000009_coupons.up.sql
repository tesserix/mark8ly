-- 000009_coupons.up.sql
-- Marketing M1: Coupons + coupon usage tracking.

CREATE TABLE coupons (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    code            VARCHAR(50)   NOT NULL,
    title           VARCHAR(200)  NOT NULL,
    description     TEXT,
    type            VARCHAR(20)   NOT NULL,  -- 'percentage', 'fixed_amount', 'free_shipping'
    value           NUMERIC(12,2) NOT NULL,  -- percent value or fixed amount
    currency_code   CHAR(3),                 -- required for fixed_amount
    min_purchase    NUMERIC(12,2),           -- NULL = no minimum
    max_discount    NUMERIC(12,2),           -- cap for percentage coupons
    usage_limit     INT,                     -- NULL = unlimited total uses
    per_customer    INT           NOT NULL DEFAULT 1,
    target_type     VARCHAR(20)   NOT NULL DEFAULT 'all', -- 'all', 'products', 'categories'
    target_ids      UUID[],                  -- product or category IDs when targeted
    stackable       BOOLEAN       NOT NULL DEFAULT false,
    starts_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    ends_at         TIMESTAMPTZ,             -- NULL = no expiry
    status          VARCHAR(20)   NOT NULL DEFAULT 'active', -- 'active', 'disabled', 'expired'
    usage_count     INT           NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, code)
);
CREATE INDEX coupons_store_status_idx ON coupons (store_id, status);

CREATE TABLE coupon_usage (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    coupon_id       UUID          NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    order_id        UUID          NOT NULL REFERENCES orders(id),
    customer_email  VARCHAR(300)  NOT NULL,
    discount_amount NUMERIC(12,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX coupon_usage_coupon_idx ON coupon_usage (coupon_id);
CREATE INDEX coupon_usage_email_idx ON coupon_usage (coupon_id, customer_email);
