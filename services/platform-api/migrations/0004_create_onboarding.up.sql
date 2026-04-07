-- Onboarding sessions hold the in-progress state of a merchant signing up
-- before their tenant row exists. The session is created the first time a
-- visitor lands on /start, persists draft data across page refreshes, and
-- terminates by either:
--   - completing → spawns a tenant row + FGA membership tuple
--   - expiring   → garbage-collected after 30 days

CREATE TABLE onboarding_sessions (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(320) NOT NULL,
    -- draft is the wizard's accumulated form state (business name, country,
    -- currency, etc). Stored as JSONB so it can evolve without migrations.
    draft           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status          VARCHAR(20)  NOT NULL DEFAULT 'in_progress',
    -- email_verified_at is set when the OTP flow succeeds. Completion
    -- requires this to be non-null.
    email_verified_at TIMESTAMPTZ,
    -- tenant_id is set when the session completes. Until then it's NULL.
    tenant_id       UUID,
    completed_at    TIMESTAMPTZ,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT onboarding_sessions_status_valid
        CHECK (status IN ('in_progress', 'verifying', 'completed', 'expired', 'abandoned')),
    CONSTRAINT onboarding_sessions_tenant_fk
        FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE SET NULL,
    CONSTRAINT onboarding_sessions_completed_consistency
        CHECK (
            (status = 'completed' AND tenant_id IS NOT NULL AND completed_at IS NOT NULL)
            OR
            (status <> 'completed' AND tenant_id IS NULL AND completed_at IS NULL)
        )
);

CREATE INDEX idx_onboarding_sessions_email ON onboarding_sessions (email);
CREATE INDEX idx_onboarding_sessions_status ON onboarding_sessions (status);
CREATE INDEX idx_onboarding_sessions_last_activity ON onboarding_sessions (last_activity_at);
