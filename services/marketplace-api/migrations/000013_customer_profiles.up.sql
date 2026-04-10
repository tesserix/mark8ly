-- 000013_customer_profiles: Customer profiles, addresses, and orders performance index.
BEGIN;

-- Customer profiles — one row per (store, email). Auto-created on first login.
CREATE TABLE customer_profiles (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    gip_uid         VARCHAR(200),
    email           VARCHAR(300)  NOT NULL,
    first_name      VARCHAR(200),
    last_name       VARCHAR(200),
    phone           VARCHAR(40),
    avatar_url      TEXT,
    tags            TEXT[]        NOT NULL DEFAULT '{}',
    status          VARCHAR(20)   NOT NULL DEFAULT 'active',
    block_reason    VARCHAR(300),
    notes           TEXT,
    marketing_opt_in BOOLEAN      NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, email)
);
CREATE INDEX cp_store_status_idx ON customer_profiles (store_id, status);
CREATE INDEX cp_gip_uid_idx ON customer_profiles (gip_uid) WHERE gip_uid IS NOT NULL;
CREATE INDEX cp_tags_idx ON customer_profiles USING GIN (tags);

-- Customer saved addresses (for storefront /account/addresses).
CREATE TABLE customer_addresses (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    customer_id     UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    label           VARCHAR(100),
    is_default      BOOLEAN       NOT NULL DEFAULT false,
    name            VARCHAR(200)  NOT NULL,
    line1           VARCHAR(300)  NOT NULL,
    line2           VARCHAR(300),
    city            VARCHAR(200)  NOT NULL,
    region          VARCHAR(200),
    postal_code     VARCHAR(40),
    country_code    CHAR(2)       NOT NULL,
    phone           VARCHAR(40),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX ca_customer_idx ON customer_addresses (customer_id);

-- Performance index for customer stats aggregation (order_count, total_spent).
CREATE INDEX orders_store_email_idx ON orders (store_id, customer_email);

COMMIT;
