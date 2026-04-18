-- 000042_subscription_arbitrage_audit.up.sql
-- §18.8 — geo-pricing arbitrage audit. HMAC-SHA256 of IP only (raw IP never persisted).

CREATE TABLE subscription_arbitrage_audit (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id      UUID        NOT NULL REFERENCES store_subscriptions(id) ON DELETE CASCADE,
    tenant_id            UUID        NOT NULL,
    store_id             UUID        NOT NULL,
    card_country         CHAR(2),
    billing_country      CHAR(2),
    ip_country           CHAR(2),
    ip_hash              VARCHAR(64),
    resolved_price_tier  VARCHAR(20) NOT NULL,
    mismatch_reason      VARCHAR(100),
    flagged_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by          UUID,
    reviewed_at          TIMESTAMPTZ,
    resolution           VARCHAR(30) NOT NULL DEFAULT 'ongoing'
        CHECK (resolution IN ('ongoing', 'false_positive_cleared', 'reprice_developed', 'terminated')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX saa_ongoing_idx ON subscription_arbitrage_audit (flagged_at)
    WHERE resolution = 'ongoing';
CREATE INDEX saa_tenant_idx ON subscription_arbitrage_audit (tenant_id);
CREATE INDEX saa_subscription_idx ON subscription_arbitrage_audit (subscription_id);

COMMENT ON COLUMN subscription_arbitrage_audit.ip_hash IS 'HMAC-SHA256(key from Secret Manager arbitrage-ip-hmac-key, data=raw_ip); 30d rotation.';
COMMENT ON COLUMN subscription_arbitrage_audit.ip_country IS 'Derived from IP geolookup; durable join field beyond HMAC rotation window.';
