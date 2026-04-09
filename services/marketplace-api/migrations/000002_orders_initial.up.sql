-- 000002_orders_initial.up.sql
-- Orders slice 1: orders, order_items, order_addresses, order_events,
-- returns, return_items, abandoned_carts, document_number_seq.
-- Spec: docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md §4
--
-- NOTE: No outbox or pending_events table is created here. Orders writes to
-- the existing shared outbox_events table (products-owned) via new aggregate
-- and event_type constants — see the Orders M2 plan.

BEGIN;

-- orders — one row per checkout. See spec §4.
CREATE TABLE orders (
    id                  uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid            NOT NULL,
    store_id            uuid            NOT NULL,
    order_number        varchar(40)     NOT NULL,
    idempotency_key     varchar(100)    NOT NULL,       -- inline, for checkout-create only
    customer_id         uuid,                           -- null for guests
    customer_email      varchar(320)    NOT NULL,
    customer_name       varchar(200),
    status              varchar(20)     NOT NULL DEFAULT 'pending',
    payment_status      varchar(20)     NOT NULL DEFAULT 'pending',
    fulfillment_status  varchar(20)     NOT NULL DEFAULT 'unfulfilled',
    subtotal            numeric(12, 2)  NOT NULL,
    shipping_total      numeric(12, 2)  NOT NULL DEFAULT 0,
    tax_total           numeric(12, 2)  NOT NULL DEFAULT 0,
    discount_total      numeric(12, 2)  NOT NULL DEFAULT 0,
    grand_total         numeric(12, 2)  NOT NULL,
    refunded_amount     numeric(12, 2)  NOT NULL DEFAULT 0,
    currency_code       char(3)         NOT NULL,
    payment_provider    varchar(40),
    payment_intent_id   varchar(200),
    notes               text,
    placed_at           timestamptz     NOT NULL DEFAULT now(),
    cancelled_at        timestamptz,
    fulfilled_at        timestamptz,
    created_at          timestamptz     NOT NULL DEFAULT now(),
    updated_at          timestamptz     NOT NULL DEFAULT now(),
    deleted_at          timestamptz,

    CONSTRAINT orders_number_per_store_unique        UNIQUE (store_id, order_number),
    CONSTRAINT orders_idempotency_per_store_unique   UNIQUE (store_id, idempotency_key),
    -- 'refunded' is DELIBERATELY absent from status; money state lives on payment_status.
    CONSTRAINT orders_status_valid                   CHECK (status IN (
        'pending','confirmed','fulfilled','cancelled'
    )),
    CONSTRAINT orders_payment_status_valid           CHECK (payment_status IN (
        'pending','authorized','paid','failed','refunded','partially_refunded'
    )),
    CONSTRAINT orders_fulfillment_status_valid       CHECK (fulfillment_status IN (
        'unfulfilled','partial','fulfilled'
    )),
    CONSTRAINT orders_grand_total_non_negative       CHECK (grand_total >= 0),
    CONSTRAINT orders_subtotal_non_negative          CHECK (subtotal >= 0),
    CONSTRAINT orders_refunded_non_negative          CHECK (refunded_amount >= 0),
    CONSTRAINT orders_refunded_not_exceeding_total   CHECK (refunded_amount <= grand_total),
    CONSTRAINT orders_currency_format                CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT orders_cancelled_requires_timestamp   CHECK (
        (status = 'cancelled' AND cancelled_at IS NOT NULL) OR status <> 'cancelled'
    )
);

-- Hot-path indexes. No tenant-only index: every real query is store-scoped.
CREATE INDEX orders_store_placed_idx
    ON orders (store_id, placed_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX orders_store_status_placed_idx
    ON orders (store_id, status, placed_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX orders_customer_idx
    ON orders (customer_id)
    WHERE deleted_at IS NULL AND customer_id IS NOT NULL;
CREATE INDEX orders_email_idx
    ON orders (lower(customer_email))
    WHERE deleted_at IS NULL;
-- B-tree with varchar_pattern_ops supports prefix search on order_number.
-- The structured M-XXX-YYMMDD-NNNNN format only needs prefix + equality; a
-- GIN trigram index would add write amplification for no real benefit.
CREATE INDEX orders_number_idx
    ON orders (store_id, order_number varchar_pattern_ops)
    WHERE deleted_at IS NULL;

-- order_items — price snapshot, NO FK on product_id / variant_id.
-- Products can be hard-deleted without corrupting order history.
CREATE TABLE order_items (
    id                uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          uuid            NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id        uuid,                         -- deliberately unconstrained
    variant_id        uuid,                         -- deliberately unconstrained
    title_snapshot    varchar(300)    NOT NULL,
    sku_snapshot      varchar(100)    NOT NULL,
    option_summary    varchar(300),                 -- e.g. "Size: M · Color: Ink"
    unit_price        numeric(12, 2)  NOT NULL,
    quantity          integer         NOT NULL,
    line_total        numeric(12, 2)  NOT NULL,
    currency_code     char(3)         NOT NULL,
    image_url         text,                         -- snapshot, not joined
    created_at        timestamptz     NOT NULL DEFAULT now(),

    CONSTRAINT order_items_quantity_positive       CHECK (quantity > 0),
    CONSTRAINT order_items_unit_price_non_negative CHECK (unit_price >= 0),
    CONSTRAINT order_items_line_total_non_negative CHECK (line_total >= 0)
);
CREATE INDEX order_items_order_idx   ON order_items (order_id);
CREATE INDEX order_items_variant_idx ON order_items (variant_id) WHERE variant_id IS NOT NULL;

-- order_addresses — immutable snapshot. No updated_at, no trigger.
CREATE TABLE order_addresses (
    id           uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     uuid         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    kind         varchar(10)  NOT NULL,
    name         varchar(200) NOT NULL,
    line1        varchar(300) NOT NULL,
    line2        varchar(300),
    city         varchar(200) NOT NULL,
    region       varchar(200),
    postal_code  varchar(40),
    country_code char(2)      NOT NULL,
    phone        varchar(40),

    CONSTRAINT order_addresses_kind_valid       CHECK (kind IN ('shipping','billing')),
    CONSTRAINT order_addresses_country_format   CHECK (country_code ~ '^[A-Z]{2}$'),
    CONSTRAINT order_addresses_kind_per_order_unique UNIQUE (order_id, kind)
);

-- order_events — append-only audit log + state transition trail.
CREATE TABLE order_events (
    id          uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    kind        varchar(40)  NOT NULL,
    actor_id    uuid,
    actor_email varchar(320),
    payload     jsonb        NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX order_events_order_idx ON order_events (order_id, created_at DESC);

-- returns — sibling table, not a status of orders.
-- RESTRICT on order_id means you cannot hard-delete an order that has returns.
CREATE TABLE returns (
    id            uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid            NOT NULL,
    store_id      uuid            NOT NULL,
    order_id      uuid            NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    return_number varchar(40)     NOT NULL,
    status        varchar(20)     NOT NULL DEFAULT 'requested',
    reason        varchar(80),
    notes         text,
    refund_amount numeric(12, 2),
    currency_code char(3)         NOT NULL,
    requested_at  timestamptz     NOT NULL DEFAULT now(),
    received_at   timestamptz,
    refunded_at   timestamptz,
    created_at    timestamptz     NOT NULL DEFAULT now(),
    updated_at    timestamptz     NOT NULL DEFAULT now(),

    CONSTRAINT returns_status_valid           CHECK (status IN (
        'requested','approved','received','refunded','rejected'
    )),
    CONSTRAINT returns_number_per_store_unique UNIQUE (store_id, return_number),
    CONSTRAINT returns_refund_non_negative    CHECK (refund_amount IS NULL OR refund_amount >= 0),
    CONSTRAINT returns_currency_format        CHECK (currency_code ~ '^[A-Z]{3}$')
);
CREATE INDEX returns_order_idx        ON returns (order_id);
CREATE INDEX returns_store_status_idx ON returns (store_id, status);

-- return_items — RESTRICT on order_items completes the chain that makes
-- hard-delete of an order with returns IMPOSSIBLE.
CREATE TABLE return_items (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id     uuid        NOT NULL REFERENCES returns(id)     ON DELETE CASCADE,
    order_item_id uuid        NOT NULL REFERENCES order_items(id) ON DELETE RESTRICT,
    quantity      integer     NOT NULL,
    reason        varchar(80),
    CONSTRAINT return_items_quantity_positive CHECK (quantity > 0)
);
CREATE INDEX return_items_return_idx ON return_items (return_id);

-- abandoned_carts — first-class rows, NOT pending orders.
-- Upsert key is (store_id, cart_session_id); the storefront cart service
-- writes and updates these on every cart change.
CREATE TABLE abandoned_carts (
    id                 uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid           NOT NULL,
    store_id           uuid           NOT NULL,
    cart_session_id    varchar(100)   NOT NULL,
    customer_email     varchar(320),
    customer_name      varchar(200),
    customer_id        uuid,
    item_count         integer        NOT NULL,
    subtotal           numeric(12, 2) NOT NULL,
    currency_code      char(3)        NOT NULL,
    items_snapshot     jsonb          NOT NULL,
    recovery_url       text,
    last_active_at     timestamptz    NOT NULL,
    recovery_sent_at   timestamptz,
    converted_order_id uuid           REFERENCES orders(id) ON DELETE SET NULL,
    created_at         timestamptz    NOT NULL DEFAULT now(),
    updated_at         timestamptz    NOT NULL DEFAULT now(),

    CONSTRAINT abandoned_carts_subtotal_non_negative CHECK (subtotal >= 0),
    CONSTRAINT abandoned_carts_item_count_positive   CHECK (item_count > 0),
    CONSTRAINT abandoned_carts_currency_format       CHECK (currency_code ~ '^[A-Z]{3}$')
);
CREATE UNIQUE INDEX abandoned_carts_session_unique        ON abandoned_carts (store_id, cart_session_id);
CREATE INDEX        abandoned_carts_tenant_idx            ON abandoned_carts (tenant_id);
CREATE INDEX        abandoned_carts_store_last_active_idx ON abandoned_carts (store_id, last_active_at DESC);
CREATE INDEX        abandoned_carts_email_idx             ON abandoned_carts (lower(customer_email))
    WHERE customer_email IS NOT NULL;

-- document_number_seq — atomic per-store per-day counter for order AND return
-- numbers. Incremented via an INSERT ... ON CONFLICT DO UPDATE ... RETURNING
-- statement in order.NextDocumentNumber (Go helper); NEVER via SELECT FOR UPDATE.
CREATE TABLE document_number_seq (
    store_id uuid        NOT NULL,
    kind     varchar(10) NOT NULL,
    day      date        NOT NULL,
    last_seq integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (store_id, kind, day),
    CONSTRAINT document_number_seq_kind_valid CHECK (kind IN ('order','return'))
);

-- Triggers reuse the shared set_updated_at() function defined in products' 000001 migration.
CREATE TRIGGER orders_set_updated_at          BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER returns_set_updated_at         BEFORE UPDATE ON returns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER abandoned_carts_set_updated_at BEFORE UPDATE ON abandoned_carts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;
