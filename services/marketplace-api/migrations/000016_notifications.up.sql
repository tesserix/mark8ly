CREATE TABLE notification_preferences (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID    NOT NULL,
    store_id        UUID    NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    preferences     JSONB   NOT NULL DEFAULT '{"new_order": true, "low_stock": true, "return_requested": true, "payment_received": true, "review_submitted": true}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE notifications (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    type            VARCHAR(40)   NOT NULL,
    title           VARCHAR(200)  NOT NULL,
    message         TEXT,
    resource_type   VARCHAR(40),
    resource_id     UUID,
    is_read         BOOLEAN       NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX notif_store_unread_idx ON notifications (store_id, is_read, created_at DESC);
CREATE INDEX notif_store_recent_idx ON notifications (store_id, created_at DESC);
