BEGIN;

CREATE TABLE wishlists (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    customer_id     UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    product_id      UUID          NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (customer_id, product_id)
);

CREATE INDEX wishlists_customer_idx ON wishlists (customer_id);
CREATE INDEX wishlists_product_idx ON wishlists (product_id);

COMMIT;
