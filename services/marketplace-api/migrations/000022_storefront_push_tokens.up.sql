CREATE TABLE IF NOT EXISTS storefront_push_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    store_slug  VARCHAR(63) NOT NULL,
    customer_id UUID        NOT NULL,
    device_id   VARCHAR(255) NOT NULL,
    token       TEXT        NOT NULL,
    platform    VARCHAR(10) NOT NULL CHECK (platform IN ('ios', 'android')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (customer_id, device_id)
);
CREATE INDEX idx_storefront_push_tokens_store ON storefront_push_tokens (store_slug);
CREATE INDEX idx_storefront_push_tokens_customer ON storefront_push_tokens (customer_id);
