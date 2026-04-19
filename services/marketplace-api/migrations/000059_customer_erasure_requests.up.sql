-- Migration 059: customer erasure requests table (GDPR Art.17 / Art.20)
-- Append-only: no UPDATE path, no merchant-facing delete endpoint.
-- Mark8ly support reads via read-only DB role.

CREATE TABLE customer_erasure_requests (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL,
    store_id        UUID        NOT NULL,
    customer_email  TEXT        NOT NULL,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    status          TEXT        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'processed', 'rejected')),
    processed_at    TIMESTAMPTZ,
    notes           TEXT,

    -- prevent duplicate erasure requests for same customer within a store
    -- (a second request just re-sets the clock; support dedupes manually)
    CONSTRAINT cer_store_email_unique UNIQUE (store_id, customer_email)
);

CREATE INDEX cer_tenant_idx  ON customer_erasure_requests (tenant_id);
CREATE INDEX cer_store_idx   ON customer_erasure_requests (store_id);
CREATE INDEX cer_status_idx  ON customer_erasure_requests (status) WHERE status = 'pending';
