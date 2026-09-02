-- Migration 126: outbound webhooks (#562).
--
-- Consumes the existing transactional outbox rather than instrumenting new
-- events: outbox_events already carries 18 domain events written in the same
-- transaction as the mutation that produced them.
--
-- Two tables, deliberately separate from outbox_events. A merchant's dead
-- endpoint must never stall the outbox watermark publisher, whose recovery
-- semantics are documented in internal/outbox/models.go (#336).
CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID         NOT NULL,
    store_id             UUID         NOT NULL,
    url                  VARCHAR(2048) NOT NULL,
    -- Event types this subscription wants, e.g. {order.placed,order.refunded}.
    -- Values come from internal/outbox's Event* constants.
    event_types          TEXT[]       NOT NULL,
    -- HMAC signing secret. Shown to the merchant once at creation.
    secret               VARCHAR(128) NOT NULL,
    enabled              BOOLEAN      NOT NULL DEFAULT true,
    -- Set when the platform auto-disables after sustained failure, so the
    -- merchant is told WHY rather than finding a silently dead webhook.
    disabled_reason      TEXT,
    disabled_at          TIMESTAMPTZ,
    consecutive_failures INTEGER      NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- The dispatcher's hot path: "enabled subscriptions for this tenant".
CREATE INDEX IF NOT EXISTS idx_webhook_subs_tenant_enabled
    ON webhook_subscriptions (tenant_id) WHERE enabled;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id   UUID         NOT NULL REFERENCES webhook_subscriptions(id) ON DELETE CASCADE,
    outbox_event_id   UUID         NOT NULL,
    event_type        VARCHAR(64)  NOT NULL,
    aggregate_id      UUID         NOT NULL,
    status            VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts          INTEGER      NOT NULL DEFAULT 0,
    next_attempt_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_status_code  INTEGER,
    -- Truncated by the worker before insert. Surfaced to the merchant so a
    -- failing endpoint is debuggable; never logged server-side.
    last_error        TEXT,
    delivered_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- This is what makes fan-out idempotent. The dispatcher inserts with
-- ON CONFLICT DO NOTHING against it, so re-running over the same outbox
-- rows cannot double-deliver — which is how we get exactly-once fan-out
-- without coupling to the outbox publisher's transaction.
CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_deliveries_event_sub
    ON webhook_deliveries (outbox_event_id, subscription_id);

-- The worker's claim query.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_due
    ON webhook_deliveries (next_attempt_at) WHERE status = 'pending';

-- Prune scan (30-day retention on every plan, see the design doc).
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created
    ON webhook_deliveries (created_at);

-- The dispatcher's own cursor over outbox_events. Deliberately NOT the
-- publisher's watermark: the two consumers advance independently, so a
-- stalled webhook dispatch cannot hold back outbox publishing.
CREATE TABLE IF NOT EXISTS webhook_dispatch_cursor (
    id                  BOOLEAN     PRIMARY KEY DEFAULT true CHECK (id),
    last_event_created  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_event_id       UUID
);
INSERT INTO webhook_dispatch_cursor (id) VALUES (true) ON CONFLICT DO NOTHING;
