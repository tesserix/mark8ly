-- 000012_campaigns.up.sql

CREATE TABLE customer_segments (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID         NOT NULL,
    store_id    UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    rules       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    member_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE campaigns (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name            VARCHAR(200) NOT NULL,
    type            VARCHAR(20)  NOT NULL DEFAULT 'email',
    status          VARCHAR(20)  NOT NULL DEFAULT 'draft',
    subject         VARCHAR(300),
    content         TEXT,
    segment_id      UUID         REFERENCES customer_segments(id),
    coupon_id       UUID         REFERENCES coupons(id),
    scheduled_at    TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    heartbeat_at    TIMESTAMPTZ,
    total_recipients INT         NOT NULL DEFAULT 0,
    delivered       INT          NOT NULL DEFAULT 0,
    opened          INT          NOT NULL DEFAULT 0,
    clicked         INT          NOT NULL DEFAULT 0,
    converted       INT          NOT NULL DEFAULT 0,
    unsubscribed    INT          NOT NULL DEFAULT 0,
    failed          INT          NOT NULL DEFAULT 0,
    revenue         NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX campaigns_store_status_idx ON campaigns (store_id, status);

CREATE TABLE campaign_recipients (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    campaign_id     UUID         NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    customer_email  VARCHAR(300) NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    sent_at         TIMESTAMPTZ,
    opened_at       TIMESTAMPTZ,
    clicked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX cr_campaign_idx ON campaign_recipients (campaign_id);
CREATE INDEX cr_email_idx ON campaign_recipients (customer_email);
