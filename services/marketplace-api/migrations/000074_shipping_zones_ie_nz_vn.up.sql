-- 000074_shipping_zones_ie_nz_vn.up.sql
-- P18 §4.1 / §4.1.1 — seed shipping_zones for v2 country rollout (IE + NZ via
-- ShipEngine, VN via NinjaVan). Table is created here because this is the
-- first migration that references it; the existing shipping_carrier_configs
-- table is per-tenant/store and does not cover default-carrier-per-country.
--
-- AE / Aramex is deliberately not seeded — deferred to v2 per spec §2 + §25.

CREATE TABLE IF NOT EXISTS shipping_zones (
    country_code         CHAR(2)     PRIMARY KEY,
    carrier_id           VARCHAR(40) NOT NULL,
    default_service_code VARCHAR(80),
    currency             CHAR(3),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE shipping_zones IS
    'Default carrier + service mapping per ISO country code. Consumed by the
     admin shipping-zones UI to assign a default carrier when a merchant
     opens a store in that country. Per-store overrides live in
     shipping_carrier_configs.';

INSERT INTO shipping_zones (country_code, carrier_id, default_service_code, currency)
VALUES
    ('IE', 'shipengine', 'an_post_parcel',    'EUR'),
    ('NZ', 'shipengine', 'nz_post_tracked',   'NZD'),
    ('VN', 'ninjavan',   'ninjavan_standard', 'VND')
ON CONFLICT (country_code) DO NOTHING;
