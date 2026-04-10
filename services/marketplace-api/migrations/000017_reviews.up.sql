BEGIN;

CREATE TABLE reviews (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    product_id          UUID          NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    customer_profile_id UUID          REFERENCES customer_profiles(id) ON DELETE SET NULL,
    customer_name       VARCHAR(200)  NOT NULL,
    customer_email      VARCHAR(300)  NOT NULL,
    rating              INT           NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title               VARCHAR(300),
    content             TEXT          NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    verified_purchase   BOOLEAN       NOT NULL DEFAULT false,
    featured            BOOLEAN       NOT NULL DEFAULT false,
    helpful_count       INT           NOT NULL DEFAULT 0,
    not_helpful_count   INT           NOT NULL DEFAULT 0,
    published_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, product_id, customer_email)
);
CREATE INDEX reviews_product_status_idx ON reviews (product_id, status);
CREATE INDEX reviews_store_status_idx ON reviews (store_id, status);
CREATE INDEX reviews_customer_idx ON reviews (customer_email, store_id);

CREATE TABLE review_media (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    url             TEXT          NOT NULL,
    alt             VARCHAR(300),
    position        INT           NOT NULL DEFAULT 0,
    media_type      VARCHAR(20)   NOT NULL DEFAULT 'image',
    width           INT,
    height          INT,
    file_size       INT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX rm_review_idx ON review_media (review_id);

CREATE TABLE review_replies (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    author_type     VARCHAR(20)   NOT NULL,
    author_name     VARCHAR(200)  NOT NULL,
    author_email    VARCHAR(300),
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX rr_review_idx ON review_replies (review_id);

CREATE TABLE review_reactions (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id           UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    customer_profile_id UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    reaction            VARCHAR(20)   NOT NULL,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (review_id, customer_profile_id)
);

COMMIT;
