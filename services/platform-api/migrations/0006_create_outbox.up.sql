-- Transactional outbox for side effects that must be guaranteed but can't be
-- run inside the originating DB transaction (network calls, external APIs).
--
-- The canonical use case for Mark8ly: writing OpenFGA tuples after a tenant
-- is created during onboarding completion. The DB tx commits the tenant row
-- AND an outbox row in one shot; a background drainer ships the outbox row
-- to OpenFGA. If the drainer crashes / OpenFGA is down, the row stays in
-- the outbox and is retried on the next drainer tick.
--
-- This is the fix for auth-bug #2 (FGA tuple race) and #3 (no retry on
-- FGA failure) from docs/planning/auth-bugs.md.
--
-- Payload is JSONB so the same outbox table works for any future side effect
-- (Pub/Sub publish, webhook fan-out, email send) without schema churn.

CREATE TABLE outbox_events (
    id              BIGSERIAL    PRIMARY KEY,
    -- kind is the dispatch key the drainer uses to pick a handler.
    kind            VARCHAR(64)  NOT NULL,
    -- payload is the handler-specific data (e.g. {"user_id": "...", "tenant_id": "..."}).
    payload         JSONB        NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts        SMALLINT     NOT NULL DEFAULT 0,
    last_error      TEXT,
    next_attempt_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,

    CONSTRAINT outbox_events_status_valid
        CHECK (status IN ('pending', 'in_flight', 'completed', 'dead'))
);

-- Drainer query: WHERE status='pending' AND next_attempt_at <= now() ORDER BY id LIMIT N
CREATE INDEX idx_outbox_pending ON outbox_events (status, next_attempt_at)
    WHERE status = 'pending';
