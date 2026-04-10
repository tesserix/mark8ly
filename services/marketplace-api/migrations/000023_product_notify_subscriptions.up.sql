CREATE TABLE IF NOT EXISTS product_notify_subscriptions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    store_slug  VARCHAR(63) NOT NULL,
    customer_id UUID        NOT NULL,
    product_id  UUID        NOT NULL,
    notify_type VARCHAR(20) NOT NULL CHECK (notify_type IN ('back_in_stock', 'price_drop')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (customer_id, product_id, notify_type)
);
CREATE INDEX idx_product_notify_subs_product ON product_notify_subscriptions (product_id);
CREATE INDEX idx_product_notify_subs_store ON product_notify_subscriptions (store_slug);
