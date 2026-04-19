-- 000068_enterprise_api_keys.up.sql
-- §18.4 Enterprise API keys. 32-byte CSPRNG → base58, prefixed mk8_live_.
-- Stored as bcrypt hashes; first 8 chars of the base58 body are kept
-- plaintext as `key_prefix` for O(log n) lookup. Rotation overlap is
-- 24h (revoked_at = now() + 24h on the rotated row).

CREATE TABLE IF NOT EXISTS enterprise_api_keys (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID         NOT NULL,
    store_id            UUID         NOT NULL,
    key_prefix          VARCHAR(8)   NOT NULL,
    key_hash            VARCHAR(60)  NOT NULL,
    scopes              JSONB        NOT NULL DEFAULT '[]'::jsonb,
    rate_limit_per_min  INTEGER      NOT NULL DEFAULT 100
                        CHECK (rate_limit_per_min > 0 AND rate_limit_per_min <= 10000),
    label               VARCHAR(100) NOT NULL,
    created_by          UUID         NOT NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    revoked_at          TIMESTAMPTZ,
    revoked_reason      VARCHAR(50),
    last_used_at        TIMESTAMPTZ,
    last_used_ip_hash   VARCHAR(64),
    rotation_replaces   UUID REFERENCES enterprise_api_keys(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS eak_tenant_prefix_uniq
    ON enterprise_api_keys (tenant_id, key_prefix);

CREATE INDEX IF NOT EXISTS eak_store_active_idx
    ON enterprise_api_keys (store_id) WHERE revoked_at IS NULL;

COMMENT ON COLUMN enterprise_api_keys.key_prefix IS
    'First 8 chars of the base58 body (excludes the mk8_live_ prefix). Plaintext for O(log n) lookup.';
COMMENT ON COLUMN enterprise_api_keys.revoked_at IS
    'NULL = active. Set to now() on revoke. Set to now()+24h on rotation (overlap window).';
