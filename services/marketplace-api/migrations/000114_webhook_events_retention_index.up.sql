-- 000114 — index supporting the webhook_events retention prune (#440).
--
-- WHY. webhook_events.payload stores the provider's raw event body verbatim
-- (internal/handlers/storefront/webhooks.go), which for Stripe carries
-- billing_details.email, shipping.address and customer_details. The table has
-- no tenant_id, store_id, order_id or customer link — see the DDL in
-- 000008_payments_shipping_tax.up.sql — so no erasure or purge path can scope
-- a delete to a person or a tenant (internal/tenantpurge/purge.go:21-25 says
-- so explicitly). Age is the only axis this table exposes, so retention is
-- enforced by an age-based prune cron: internal/webhookprune.
--
-- THE POLICY THIS INDEX SERVES, in the table's own vocabulary:
--   * status = 'processed'  -> 30 days. Set by the UPDATE in webhooks.go once
--     processEvent has run; the payload has already done its job.
--   * everything else       -> 90 days. In practice that is 'received', the
--     value the INSERT writes. This table has NO 'failed' status — 'received'
--     and 'processed' are the only two values any code writes — so the
--     long-retention class is the UNPROCESSED/stuck case: accepted and stored
--     but never carried to completion, and therefore the one someone may
--     still need to inspect or replay. The predicate is written as an
--     exclusion (status <> 'processed') so a future provider integration
--     adding a third status inherits the SAFER, longer window by default.
--
-- WHY AN INDEX AT ALL. Before this migration the table carried only its
-- primary key and the UNIQUE (provider, provider_event_id) idempotency
-- constraint — nothing on created_at and nothing on status. The prune's
-- `WHERE status ... AND created_at < ?` would seq-scan a table that receives
-- every payment webhook in the estate. It is empty in dev and will not be in
-- production.
--
-- COLUMN ORDER. status first, created_at second: status is the equality
-- predicate and created_at the range, so this ordering lets one index scan
-- serve both classes' queries and return rows already ordered by age, which
-- is what the prune's LIMIT batching wants.
CREATE INDEX IF NOT EXISTS idx_webhook_events_status_created_at
    ON webhook_events (status, created_at);

COMMENT ON INDEX idx_webhook_events_status_created_at IS
    'Supports the webhook_events retention prune (#440): processed rows 30 days, unprocessed (status <> ''processed'', i.e. ''received'') 90 days.';
