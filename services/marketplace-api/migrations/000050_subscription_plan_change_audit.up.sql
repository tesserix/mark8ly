-- 000050_subscription_plan_change_audit.up.sql
CREATE TABLE subscription_plan_change_audit (
    id                       UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                UUID            NOT NULL,
    store_id                 UUID            NOT NULL,
    stripe_subscription_id   VARCHAR(64),
    stripe_invoice_id        VARCHAR(64),
    from_plan                VARCHAR(30)     NOT NULL,
    to_plan                  VARCHAR(30)     NOT NULL,
    from_period              VARCHAR(10)     NOT NULL,
    to_period                VARCHAR(10)     NOT NULL,
    action                   VARCHAR(40)     NOT NULL,
    billing_currency         CHAR(3)         NOT NULL,
    proration_cents          BIGINT,
    actor                    VARCHAR(128)    NOT NULL,
    reason                   VARCHAR(256),
    effective_at             TIMESTAMPTZ     NOT NULL,
    created_at               TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT spca_action_check CHECK (action IN (
        'upgrade_committed',
        'downgrade_scheduled',
        'downgrade_committed',
        'downgrade_blocked_over_quota',
        'period_switch_committed'
    ))
);

CREATE INDEX spca_store_idx   ON subscription_plan_change_audit (store_id, created_at DESC);
CREATE INDEX spca_tenant_idx  ON subscription_plan_change_audit (tenant_id, created_at DESC);
CREATE INDEX spca_action_idx  ON subscription_plan_change_audit (action, created_at DESC);

REVOKE UPDATE, DELETE ON subscription_plan_change_audit FROM PUBLIC;
