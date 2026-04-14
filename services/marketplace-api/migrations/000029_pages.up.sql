CREATE TABLE IF NOT EXISTS pages (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL,
    slug            VARCHAR(63)  NOT NULL,
    title           VARCHAR(200) NOT NULL,
    body            TEXT         NOT NULL DEFAULT '',
    seo_title       VARCHAR(200),
    seo_description VARCHAR(300),
    published       BOOLEAN      NOT NULL DEFAULT true,
    sort_order      INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS pages_store_slug_idx ON pages (store_id, slug);
CREATE INDEX IF NOT EXISTS pages_tenant_id_idx        ON pages (tenant_id);
CREATE INDEX IF NOT EXISTS pages_store_published_idx  ON pages (store_id, published);
