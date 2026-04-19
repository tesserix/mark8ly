-- 000071_tenant_sso_user_mappings.up.sql
-- §12 JIT SSO user bindings: external NameID / OIDC sub -> internal user.
-- citext makes email lookups case-insensitive without scatter-casting.

CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE tenant_sso_user_mappings (
    tenant_id         UUID NOT NULL,
    external_user_id  TEXT NOT NULL,          -- SAML NameID / OIDC sub
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
