-- 000070_tenant_sso_configs.up.sql
-- §12 Per-tenant SSO provider config (SAML 2.0 / OIDC via GIP).
-- One row per tenant; `metadata` is an opaque JSONB bag whose schema
-- depends on `provider`. `gip_provider_id` records the deterministic
-- Firebase/GIP provider id so re-uploads are idempotent.

CREATE TYPE sso_provider_kind AS ENUM ('saml', 'oidc');

CREATE TABLE tenant_sso_configs (
    tenant_id       UUID PRIMARY KEY,
    provider        sso_provider_kind NOT NULL,
    -- SAML { idp_entity_id, idp_acs_url, idp_cert_pem, sp_entity_id, sp_acs_url }
    -- OIDC { issuer, client_id, client_secret_ref, discovery_url, redirect_uri, scopes }
    -- client_secret_ref is a Secret Manager path, NEVER a raw secret.
    metadata        JSONB NOT NULL,
    -- { "email":"claims.email", "firstName":"claims.given_name", "groups":"claims.groups" }
    attr_mapping    JSONB NOT NULL DEFAULT '{}'::jsonb,
    gip_provider_id TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tenant_sso_configs_metadata_required CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT tenant_sso_configs_attr_mapping_object CHECK (jsonb_typeof(attr_mapping) = 'object')
);

CREATE INDEX idx_tenant_sso_configs_enabled ON tenant_sso_configs(enabled) WHERE enabled = true;

COMMENT ON TABLE tenant_sso_configs IS 'Per-tenant SSO provider config (SAML 2.0 / OIDC via GIP). §12.';
