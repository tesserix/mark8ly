CREATE TABLE store_subscriptions (
    id                      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID          NOT NULL,
    store_id                UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    stripe_customer_id      VARCHAR(100)  NOT NULL,
    stripe_subscription_id  VARCHAR(100),
    plan                    VARCHAR(30)   NOT NULL DEFAULT 'free',
    status                  VARCHAR(30)   NOT NULL DEFAULT 'active',
    current_period_start    TIMESTAMPTZ,
    current_period_end      TIMESTAMPTZ,
    cancel_at_period_end    BOOLEAN       NOT NULL DEFAULT false,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);
CREATE INDEX ss_stripe_cust_idx ON store_subscriptions (stripe_customer_id);
