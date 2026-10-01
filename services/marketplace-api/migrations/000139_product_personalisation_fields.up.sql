-- Migration 139: buyer-personalised products — the catalog half (#962).
--
-- Fields hang off the PRODUCT, not the variant. A shirt in 3 sizes x 2
-- colours is 6 variants and ONE "upload your artwork" field; attaching
-- fields to variants would multiply the merchant's work by six and buy
-- nothing. See docs/superpowers/specs/2026-10-01-custom-products-design.md
-- decision 1.
--
-- Composite FK to products (id, store_id), copying product_variants
-- (spec §14.4): store_id can then never drift from the product's.
--
-- Nothing buyer-facing lands here. This migration creates the shape a
-- merchant authors; the upload and order-snapshot tables are #963/#967.
CREATE TABLE product_personalisation_fields (
    id         uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid         NOT NULL,
    store_id   uuid         NOT NULL,
    product_id uuid         NOT NULL,
    -- Stable machine key. Snapshotted onto the order line at checkout, so
    -- renaming the label later does not rewrite history.
    key        varchar(60)  NOT NULL,
    label      varchar(120) NOT NULL,
    kind       varchar(20)  NOT NULL,
    required   boolean      NOT NULL DEFAULT false,
    position   integer      NOT NULL DEFAULT 0,
    help_text  varchar(300),

    -- Kind-specific constraints. Each is NULL for every kind it does not
    -- apply to, enforced by the CHECKs below rather than by convention —
    -- a max_length on an image field is a bug, not a harmless extra.
    max_length integer,        -- text, textarea
    max_images integer,        -- image
    min_px     integer,        -- image: drives the buyer's resolution warning
    -- image: optional merchant mockup the buyer's artwork is composited
    -- into, and the rectangle it occupies as {x,y,w,h} percentages.
    mockup_storage_key text,
    print_area jsonb,
    -- checkbox: what ticking it adds to the line. Absolute money in the
    -- store's currency, never a percentage — a percentage of a variant
    -- price that itself varies is a support ticket waiting to happen.
    price_delta numeric(12,2),

    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT personalisation_fields_product_store_fk
        FOREIGN KEY (product_id, store_id)
        REFERENCES products (id, store_id) ON DELETE CASCADE,
    CONSTRAINT personalisation_fields_key_per_product_unique
        UNIQUE (product_id, key),
    CONSTRAINT personalisation_fields_kind_valid
        CHECK (kind IN ('image','text','textarea','select','checkbox')),
    -- Keys are referenced in payloads and snapshots; keep them boring.
    CONSTRAINT personalisation_fields_key_format
        CHECK (key ~ '^[a-z0-9]+(?:_[a-z0-9]+)*$'),
    CONSTRAINT personalisation_fields_label_not_blank
        CHECK (length(trim(label)) > 0),
    CONSTRAINT personalisation_fields_max_length_only_text
        CHECK (max_length IS NULL OR (kind IN ('text','textarea') AND max_length > 0)),
    CONSTRAINT personalisation_fields_image_only_cols
        CHECK (
            (kind = 'image')
            OR (max_images IS NULL AND min_px IS NULL
                AND mockup_storage_key IS NULL AND print_area IS NULL)
        ),
    CONSTRAINT personalisation_fields_max_images_positive
        CHECK (max_images IS NULL OR max_images > 0),
    CONSTRAINT personalisation_fields_min_px_positive
        CHECK (min_px IS NULL OR min_px > 0),
    CONSTRAINT personalisation_fields_price_delta_only_checkbox
        CHECK (price_delta IS NULL OR kind = 'checkbox'),
    CONSTRAINT personalisation_fields_price_delta_non_negative
        CHECK (price_delta IS NULL OR price_delta >= 0)
);

-- The authoring and (later) checkout-validation read: "every field on
-- this product, in order".
CREATE INDEX personalisation_fields_product_idx
    ON product_personalisation_fields (product_id, position);
CREATE INDEX personalisation_fields_tenant_idx
    ON product_personalisation_fields (tenant_id);

COMMENT ON TABLE product_personalisation_fields IS
    'What a buyer may supply on a product: an image, some text, a choice. Catalog side only. #962.';

-- ------------------------------------------------------------
-- product_personalisation_options — the values of a `select`
--
-- No tenant_id/store_id: the row is meaningless without its field and
-- cascades from it. Scope is reached through the parent, which is how
-- product_option_values does it too.
-- ------------------------------------------------------------
CREATE TABLE product_personalisation_options (
    id       uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    field_id uuid         NOT NULL
        REFERENCES product_personalisation_fields(id) ON DELETE CASCADE,
    value    varchar(200) NOT NULL,
    label    varchar(200) NOT NULL,
    -- What choosing this option adds to the line. The ONLY source of a
    -- select's money: checkout reads it by id and never trusts the
    -- request (see checkout_reprice.go).
    price_delta numeric(12,2) NOT NULL DEFAULT 0,
    position integer      NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT personalisation_options_value_per_field_unique
        UNIQUE (field_id, value),
    CONSTRAINT personalisation_options_value_not_blank
        CHECK (length(trim(value)) > 0),
    CONSTRAINT personalisation_options_label_not_blank
        CHECK (length(trim(label)) > 0),
    CONSTRAINT personalisation_options_price_delta_non_negative
        CHECK (price_delta >= 0)
);
CREATE INDEX personalisation_options_field_idx
    ON product_personalisation_options (field_id, position);

COMMENT ON TABLE product_personalisation_options IS
    'Values of a select-kind personalisation field, each with the money it adds. #962.';
