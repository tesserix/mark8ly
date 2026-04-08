-- Roll back Phase Q: restore the columns on tenants and copy data
-- back from the default ("first") store per tenant. Data from any
-- additional stores created after Phase Q landed IS LOST on
-- rollback — this is the trade-off for a destructive migration.

ALTER TABLE tenants ADD COLUMN slug VARCHAR(63);
ALTER TABLE tenants ADD COLUMN country_code CHAR(2);
ALTER TABLE tenants ADD COLUMN currency_code CHAR(3);
ALTER TABLE tenants ADD COLUMN timezone VARCHAR(64);

UPDATE tenants t
SET
    slug          = s.slug,
    country_code  = s.country_code,
    currency_code = s.currency_code,
    timezone      = s.timezone
FROM (
    SELECT DISTINCT ON (tenant_id) *
    FROM stores
    ORDER BY tenant_id, created_at ASC
) s
WHERE s.tenant_id = t.id;

ALTER TABLE tenants ALTER COLUMN slug SET NOT NULL;
ALTER TABLE tenants ALTER COLUMN country_code SET NOT NULL;
ALTER TABLE tenants ALTER COLUMN currency_code SET NOT NULL;
ALTER TABLE tenants ALTER COLUMN timezone SET NOT NULL;

ALTER TABLE tenants ADD CONSTRAINT tenants_slug_format
    CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$');
ALTER TABLE tenants ADD CONSTRAINT tenants_country_fk
    FOREIGN KEY (country_code) REFERENCES countries(code) ON DELETE RESTRICT;
ALTER TABLE tenants ADD CONSTRAINT tenants_currency_fk
    FOREIGN KEY (currency_code) REFERENCES currencies(code) ON DELETE RESTRICT;
ALTER TABLE tenants ADD CONSTRAINT tenants_timezone_fk
    FOREIGN KEY (timezone) REFERENCES timezones(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX IF NOT EXISTS tenants_slug_unique ON tenants (slug);

DROP TABLE stores;
