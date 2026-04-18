-- 000044_billing_archive.up.sql
-- §23.2 — 7-year billing retention for GDPR legal-obligation basis. No PII beyond what tax/audit law requires.

CREATE TABLE billing_archive (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    original_store_id    UUID         NOT NULL,
    original_tenant_id   UUID         NOT NULL,
    business_name        VARCHAR(500) NOT NULL,
    tax_id               VARCHAR(50),
    tax_id_country       CHAR(2),
    billing_country      CHAR(2),
    billing_currency     CHAR(3),
    stripe_customer_id   VARCHAR(100) NOT NULL,
    all_invoices         JSONB        NOT NULL,
    total_revenue_usd    NUMERIC(12,2),
    hard_deleted_at      TIMESTAMPTZ  NOT NULL,
    archive_expires_at   TIMESTAMPTZ  NOT NULL
);

CREATE INDEX ba_expires_idx      ON billing_archive (archive_expires_at);
CREATE INDEX ba_stripe_cust_idx  ON billing_archive (stripe_customer_id);
CREATE INDEX ba_tenant_idx       ON billing_archive (original_tenant_id);

COMMENT ON TABLE billing_archive IS '§23.2 — retained 7 years after hard-delete under legal-obligation basis.';
