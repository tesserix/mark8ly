-- Migration 000011: Loyalty program tables (Marketing M3)

CREATE TABLE loyalty_programs (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    is_active           BOOLEAN       NOT NULL DEFAULT false,
    points_per_dollar   NUMERIC(5,2)  NOT NULL DEFAULT 1.00,
    points_currency     VARCHAR(20)   NOT NULL DEFAULT 'points',
    signup_bonus        INT           NOT NULL DEFAULT 0,
    referral_bonus      INT           NOT NULL DEFAULT 0,
    referee_bonus       INT           NOT NULL DEFAULT 0,
    point_expiry_days   INT,
    min_redeem_points   INT           NOT NULL DEFAULT 100,
    points_value        NUMERIC(8,4)  NOT NULL DEFAULT 0.01,
    tiers               JSONB         NOT NULL DEFAULT '[]'::jsonb,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE customer_loyalties (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    customer_email  VARCHAR(300)  NOT NULL,
    customer_name   VARCHAR(200),
    points_balance  INT           NOT NULL DEFAULT 0,
    lifetime_points INT           NOT NULL DEFAULT 0,
    tier            VARCHAR(50)   NOT NULL DEFAULT 'bronze',
    referral_code   VARCHAR(20)   NOT NULL,
    referred_by     UUID,
    enrolled_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, customer_email),
    CHECK (points_balance >= 0)
);
CREATE INDEX cl_store_tier_idx ON customer_loyalties (store_id, tier);
CREATE INDEX cl_referral_code_idx ON customer_loyalties (store_id, referral_code);

CREATE TABLE loyalty_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    loyalty_id      UUID          NOT NULL REFERENCES customer_loyalties(id) ON DELETE CASCADE,
    order_id        UUID,
    type            VARCHAR(20)   NOT NULL,
    points          INT           NOT NULL,
    balance_after   INT           NOT NULL,
    description     VARCHAR(200),
    adjusted_by     VARCHAR(200),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CHECK (balance_after >= 0)
);
CREATE INDEX lt_loyalty_idx ON loyalty_transactions (loyalty_id);
CREATE INDEX lt_created_at_idx ON loyalty_transactions (created_at);
CREATE INDEX lt_type_created_idx ON loyalty_transactions (type, created_at);

CREATE TABLE referrals (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    referrer_id     UUID          NOT NULL REFERENCES customer_loyalties(id),
    referee_id      UUID          NOT NULL REFERENCES customer_loyalties(id),
    status          VARCHAR(20)   NOT NULL DEFAULT 'pending',
    referrer_bonus  INT           NOT NULL DEFAULT 0,
    referee_bonus   INT           NOT NULL DEFAULT 0,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, referee_id),
    CHECK (referrer_id != referee_id)
);
