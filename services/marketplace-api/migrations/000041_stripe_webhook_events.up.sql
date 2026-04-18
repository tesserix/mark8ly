-- 000041_stripe_webhook_events.up.sql
-- §17.7: idempotency table for Stripe webhook dispatch.
-- event_id is the Stripe event.id — used as PK so INSERT ON CONFLICT is the idempotency guard.

CREATE TABLE stripe_webhook_events (
    event_id         VARCHAR(100) PRIMARY KEY,
    event_type       VARCHAR(100) NOT NULL,
    store_id         UUID,
    tenant_id        UUID,
    payload          JSONB        NOT NULL,
    received_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    processed_at     TIMESTAMPTZ,
    processing_error TEXT,
    retry_count      INT          NOT NULL DEFAULT 0,
    manual_review_required BOOLEAN NOT NULL DEFAULT false
);

-- Orphan-resolver cron (P2) scans for unprocessed events; index for that path.
CREATE INDEX swe_orphan_idx
    ON stripe_webhook_events (received_at)
    WHERE processed_at IS NULL AND store_id IS NULL AND manual_review_required = false;

-- Observability: per-event-type + per-time-window queries.
CREATE INDEX swe_type_received_idx
    ON stripe_webhook_events (event_type, received_at DESC);

-- Manual-review queue dashboard.
CREATE INDEX swe_manual_review_idx
    ON stripe_webhook_events (received_at DESC)
    WHERE manual_review_required = true;

COMMENT ON COLUMN stripe_webhook_events.event_id IS 'Stripe event.id; serves as idempotency key.';
COMMENT ON COLUMN stripe_webhook_events.store_id IS 'Resolved from stripe_customer_id -> store_subscriptions; NULL until resolved.';
COMMENT ON COLUMN stripe_webhook_events.manual_review_required IS '§17.7 — set after 6 retries (30 min); cron stops retrying.';
