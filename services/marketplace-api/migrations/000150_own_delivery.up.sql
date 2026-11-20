-- Own-courier delivery (design 2026-11-20): a store either quotes live carrier
-- rates ('carrier', today's behaviour) or offers its own flat-rate delivery
-- options and ships with its own couriers ('own'). Existing stores stay 'carrier'.
ALTER TABLE stores
    ADD COLUMN shipping_mode varchar(10) NOT NULL DEFAULT 'carrier'
        CHECK (shipping_mode IN ('carrier', 'own')),
    ADD COLUMN own_free_shipping_min numeric(12,2);

CREATE TABLE store_delivery_options (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL,
    store_id   uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name       varchar(40) NOT NULL,
    price      numeric(12,2) NOT NULL,
    estimate   varchar(40),
    enabled    boolean NOT NULL DEFAULT true,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX store_delivery_options_store_position_idx ON store_delivery_options (store_id, position);

CREATE TABLE store_couriers (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL,
    store_id              uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name                  varchar(40) NOT NULL,
    tracking_url_template varchar(500),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX store_couriers_store_idx ON store_couriers (store_id);

-- What the buyer chose at checkout, kept on the order so later edits to the
-- store's options never rewrite history.
ALTER TABLE orders
    ADD COLUMN shipping_option_name varchar(40),
    ADD COLUMN shipping_option_estimate varchar(40);

ALTER TABLE shipments
    ALTER COLUMN carrier TYPE varchar(40),
    ADD COLUMN courier_id uuid,
    ADD COLUMN courier_name varchar(40),
    ADD COLUMN tracking_url text;

-- One live own-delivery shipment per order; cancelled ones may be replaced.
CREATE UNIQUE INDEX shipments_one_active_own_per_order
    ON shipments (order_id) WHERE carrier = 'own' AND status <> 'cancelled';
