-- Migration 127: store-scoped fan-out, and a sweep watermark for the
-- dispatcher's late-commit safety net (#562).
--
-- (1) webhook_subscriptions.store_id is NOT NULL and scopes every
-- merchant-facing read (ListForStore, ownedSubscription) and the admin UI,
-- but the fan-out query that decides who RECEIVES a delivery matched on
-- tenant_id alone. On a paid plan (plangate FeatureStores grants 2/5/10
-- stores) a webhook registered on Store A therefore also received Store B's
-- events — and because the payload is identifier-only the merchant could
-- not even tell: their follow-up API fetch just 404s.
--
-- SubscriptionRepo.MatchingEvent now filters on (tenant_id, store_id), so
-- the partial index behind it has to carry store_id too. 000126 has already
-- been applied, so this replaces its index rather than editing it.
DROP INDEX IF EXISTS idx_webhook_subs_tenant_enabled;

CREATE INDEX IF NOT EXISTS idx_webhook_subs_tenant_store_enabled
    ON webhook_subscriptions (tenant_id, store_id) WHERE enabled;

-- (2) A second watermark on the dispatch cursor.
--
-- outbox_events.created_at is stamped at INSERT, but the row is invisible
-- until the business transaction commits — and the enqueue is not the last
-- statement before commit. So a row stamped EARLIER can become visible
-- LATER than one stamped after it, and the (created_at, id) cursor, having
-- already walked past, would never select it again: silent, permanent
-- delivery loss. Replica clock skew produces the same shape, since
-- created_at comes from the pod clock.
--
-- last_event_* stays the prompt forward cursor. swept_* trails it through
-- the region old enough that every transaction touching it has certainly
-- committed (now() - webhook.DispatchLookback), so nothing can be missed
-- there. Fan-out is idempotent, so the sweep's re-reads cost nothing.
--
-- Seeded from last_event_* rather than from epoch: a fresh sweep watermark
-- at epoch would walk the entire history of outbox_events and fan months of
-- old orders out to live merchant endpoints.
ALTER TABLE webhook_dispatch_cursor
    ADD COLUMN IF NOT EXISTS swept_created TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS swept_id      UUID;

UPDATE webhook_dispatch_cursor
   SET swept_created = last_event_created,
       swept_id      = last_event_id;
