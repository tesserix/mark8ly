-- 000107_inbox_action_idempotency.up.sql
-- Inbox action execution (#281a).
--
-- POST /admin/inbox/:kind/:id/actions/:actionId requires an Idempotency-Key
-- for DESTRUCTIVE actions: a queue action retried after a client timeout must
-- not fire twice. The underlying domain writes cannot provide this on their
-- own — migration.Repository.Approve, for instance, matches on
-- `status = 'pending'` and returns ErrNotFound on a second call, which is a
-- failure response for an operation that in fact succeeded. The console would
-- show an error for work that was done.
--
-- The unique constraint IS the check, exactly as platform_request_nonces does
-- it (000101): mark8ly runs multiple replicas, so an in-memory map would let a
-- retry routed to another pod through.
--
-- outcome stores what the first attempt returned so a replay can answer
-- identically rather than merely "already done". Keyed by the key ALONE, not
-- by (key, item): reusing one key against a different item is a client bug,
-- and answering the first call's result for it is safer than performing a
-- second, different write under a key the client believes it already spent.
CREATE TABLE IF NOT EXISTS inbox_action_idempotency (
    idempotency_key TEXT PRIMARY KEY,
    kind            TEXT        NOT NULL,
    item_id         TEXT        NOT NULL,
    action_id       TEXT        NOT NULL,
    operator_id     TEXT        NOT NULL,
    outcome         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);

-- Sweeping expired rows is the only query that does not go by primary key.
CREATE INDEX IF NOT EXISTS idx_inbox_action_idempotency_expires_at
    ON inbox_action_idempotency (expires_at);
