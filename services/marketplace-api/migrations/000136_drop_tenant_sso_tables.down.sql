-- 000136_drop_tenant_sso_tables.down.sql
-- Recreates the dropped shapes for rollback safety. Note: this restores
-- the tenant_sso_configs shape as of 000133 (post gip_provider_id drop),
-- not the original 000070 shape — the column removal is not reversed
-- here since 000133's own down migration is what restores it.

CREATE TYPE sso_provider_kind AS ENUM ('saml', 'oidc');

CREATE TABLE tenant_sso_configs (
    tenant_id    UUID PRIMARY KEY,
    provider     sso_provider_kind NOT NULL,
    metadata     JSONB NOT NULL,
    attr_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled      BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tenant_sso_configs_metadata_required CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT tenant_sso_configs_attr_mapping_object CHECK (jsonb_typeof(attr_mapping) = 'object')
);

CREATE INDEX idx_tenant_sso_configs_enabled ON tenant_sso_configs(enabled) WHERE enabled = true;

COMMENT ON TABLE tenant_sso_configs IS 'Per-tenant SSO provider config (SAML 2.0 / OIDC via GIP). §12.';

CREATE TABLE tenant_sso_user_mappings (
    tenant_id         UUID NOT NULL,
    external_user_id  TEXT NOT NULL,
    internal_user_id  UUID NOT NULL,
    email             CITEXT NOT NULL,
    last_login_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, external_user_id),
    CONSTRAINT tenant_sso_user_mappings_internal_unique UNIQUE (tenant_id, internal_user_id)
);

CREATE INDEX idx_tenant_sso_user_mappings_email ON tenant_sso_user_mappings(tenant_id, email);

COMMENT ON TABLE tenant_sso_user_mappings IS 'JIT SSO user bindings + last-login audit trail. §12.';
