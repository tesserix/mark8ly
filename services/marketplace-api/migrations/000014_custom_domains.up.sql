CREATE TABLE custom_domains (
    id                      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID          NOT NULL,
    store_id                UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    domain                  VARCHAR(253)  NOT NULL,
    status                  VARCHAR(20)   NOT NULL DEFAULT 'pending',
    cloudflare_zone_id      VARCHAR(100),
    cloudflare_dns_record_id VARCHAR(100),
    cf_api_token_encrypted  TEXT          NOT NULL,
    ssl_status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    verified_at             TIMESTAMPTZ,
    error_message           TEXT,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (domain)
);
CREATE INDEX cd_store_idx ON custom_domains (store_id);
