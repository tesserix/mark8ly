-- Tenants are the core domain entity. One row per merchant on Mark8ly.
--
-- Slug is the public, URL-safe identifier ({slug}.mark8ly.com,
-- {slug}-admin.mark8ly.com). Once assigned it cannot change without
-- breaking inbound URLs.

CREATE TABLE tenants (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            VARCHAR(63)  NOT NULL UNIQUE,
    name            VARCHAR(200) NOT NULL,
    owner_user_id   VARCHAR(128) NOT NULL,                  -- GIP UID of the owner
    owner_email     VARCHAR(320) NOT NULL,
    country_code    CHAR(2)      NOT NULL,
    currency_code   CHAR(3)      NOT NULL,
    timezone        VARCHAR(64)  NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT tenants_status_valid
        CHECK (status IN ('active', 'suspended', 'archived')),
    CONSTRAINT tenants_country_fk
        FOREIGN KEY (country_code) REFERENCES countries(code) ON DELETE RESTRICT,
    CONSTRAINT tenants_currency_fk
        FOREIGN KEY (currency_code) REFERENCES currencies(code) ON DELETE RESTRICT,
    CONSTRAINT tenants_timezone_fk
        FOREIGN KEY (timezone) REFERENCES timezones(id) ON DELETE RESTRICT,
    -- 3 to 63 lowercase alphanumeric chars and hyphens, no edge hyphens.
    -- Matches internal/tenant.slugPattern in service.go.
    CONSTRAINT tenants_slug_format
        CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$')
);

CREATE INDEX idx_tenants_owner_user ON tenants (owner_user_id);
CREATE INDEX idx_tenants_owner_email ON tenants (owner_email);
