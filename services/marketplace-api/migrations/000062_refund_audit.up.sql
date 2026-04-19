-- 000062_refund_audit.up.sql
-- §8 — refund audit trail: one row per issued subscription refund.

CREATE TABLE refund_audit (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id   UUID        NOT NULL,
    store_id          UUID        NOT NULL,
    tenant_id         UUID        NOT NULL,
    stripe_refund_id  VARCHAR(100) NOT NULL,
    stripe_charge_id  VARCHAR(100) NOT NULL,
    amount_minor      BIGINT      NOT NULL CHECK (amount_minor > 0),
    currency          CHAR(3)     NOT NULL,
    reason            TEXT        NOT NULL,
    -- card_fingerprint: Stripe charge.payment_method_details.card.fingerprint
    -- used for cross-subscription fraud guard (§8).
    card_fingerprint  VARCHAR(100),
    -- device_fingerprint: X-Device-Fingerprint header, logged for fraud audit.
    device_fingerprint VARCHAR(200),
    issued_by         TEXT        NOT NULL,
    issued_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- §8 card-fingerprint fraud guard: prevent refund to same card across subscriptions.
-- Partial index: only rows where fingerprint is known.
CREATE UNIQUE INDEX refund_audit_card_fp_uidx ON refund_audit (card_fingerprint)
    WHERE card_fingerprint IS NOT NULL;

CREATE INDEX refund_audit_store_idx        ON refund_audit (store_id);
CREATE INDEX refund_audit_subscription_idx ON refund_audit (subscription_id);
CREATE INDEX refund_audit_tenant_idx       ON refund_audit (tenant_id);

COMMENT ON TABLE refund_audit IS '§8 — subscription refund audit log with card-fingerprint fraud guard.';
