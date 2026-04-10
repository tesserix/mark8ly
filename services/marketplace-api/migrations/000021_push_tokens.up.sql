CREATE TABLE admin_push_tokens (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID          NOT NULL,
    store_id    UUID          NOT NULL,
    user_id     UUID          NOT NULL,
    device_id   VARCHAR(100)  NOT NULL,
    token       TEXT          NOT NULL,
    platform    VARCHAR(10)   NOT NULL CHECK (platform IN ('ios', 'android')),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (user_id, device_id),
    UNIQUE (token)
);
CREATE INDEX apt_store_idx ON admin_push_tokens (store_id);
