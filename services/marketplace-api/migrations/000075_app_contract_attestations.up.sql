-- 000075_app_contract_attestations.up.sql
-- P15 §13.2 / §14.2 — Apple Guideline 4.2.6 ack captured at add-on purchase time.
-- Mirrors business_entity_attestations (migration 000045): append-only, with both
-- a BEFORE UPDATE trigger AND a role-level DELETE revoke. Both required so that
-- neither a DROP TRIGGER nor a compromised app role can tamper with the record.

CREATE TABLE app_contract_attestations (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID        NOT NULL,
    store_id            UUID        NOT NULL,
    subscription_id     UUID        NOT NULL,
    attestation_type    VARCHAR(40) NOT NULL
        CHECK (attestation_type IN ('apple_4_2_6')),
    attested_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    attested_by_user_id UUID        NOT NULL,
    attestation_text    TEXT        NOT NULL,
    ip_address          INET,
    user_agent          TEXT,
    stripe_invoice_id   TEXT        NOT NULL UNIQUE
);

CREATE INDEX aca_store_tenant_idx ON app_contract_attestations (tenant_id, store_id);

-- Trigger: reject UPDATE. Reuses raise_immutable_exception() function created
-- by migration 000045 (business_entity_attestations). If that migration ever
-- changes, this trigger still works — the function is table-agnostic.
CREATE TRIGGER app_contract_attestations_no_update
    BEFORE UPDATE ON app_contract_attestations
    FOR EACH ROW EXECUTE FUNCTION raise_immutable_exception();

-- Role-level DELETE revoke. Matches the pattern in migration 000045:
-- handle missing role gracefully for dev DBs that don't have marketplace_user.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'marketplace_user') THEN
        REVOKE DELETE ON app_contract_attestations FROM marketplace_user;
    END IF;
    REVOKE DELETE ON app_contract_attestations FROM PUBLIC;
END$$;

COMMENT ON TABLE app_contract_attestations IS
    'Append-only per §13.2/§14.2. Records the Apple 4.2.6 acknowledgment
     captured at white-label app add-on purchase. Trigger blocks UPDATE;
     role-level revoke blocks DELETE.';
