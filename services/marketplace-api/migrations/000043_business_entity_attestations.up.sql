-- 000043_business_entity_attestations.up.sql
-- §19.3.1 — US/CA B2B entity attestation. Append-only: trigger blocks UPDATE; role revoke blocks DELETE.
-- Both required (Security finding): a DROP TRIGGER alone would bypass the UPDATE block,
-- but the DELETE revoke is at role level and closes the path.

CREATE TABLE business_entity_attestations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id         UUID        NOT NULL,
    tenant_id        UUID        NOT NULL,
    country          CHAR(2)     NOT NULL,
    checkbox_text    TEXT        NOT NULL,
    checkbox_version VARCHAR(20) NOT NULL,
    user_agent       TEXT,
    ip_hash          VARCHAR(64),
    signed_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX bea_store_idx ON business_entity_attestations (store_id);
CREATE INDEX bea_tenant_idx ON business_entity_attestations (tenant_id);

-- Trigger: reject UPDATE.
CREATE OR REPLACE FUNCTION raise_immutable_exception() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'business_entity_attestations is append-only; UPDATE rejected';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER business_entity_no_update
    BEFORE UPDATE ON business_entity_attestations
    FOR EACH ROW EXECUTE FUNCTION raise_immutable_exception();

-- Role-level DELETE revoke. The app role must NOT be able to DELETE these rows.
-- Use a DO block to handle the case where the role doesn't yet exist in a dev DB.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'marketplace_user') THEN
        REVOKE DELETE ON business_entity_attestations FROM marketplace_user;
    END IF;
    -- Also ensure PUBLIC has no DELETE grant (belt + braces).
    REVOKE DELETE ON business_entity_attestations FROM PUBLIC;
END$$;

COMMENT ON TABLE business_entity_attestations IS 'Append-only per §19.3.1. Trigger blocks UPDATE, role-level revoke blocks DELETE.';
