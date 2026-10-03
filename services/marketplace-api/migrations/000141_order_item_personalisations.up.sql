-- Migration 141: what the buyer asked for, as the order records it (#967).
--
-- EVERYTHING HERE IS A SNAPSHOT, and there is no FK back to
-- product_personalisation_fields on purpose.
--
-- An order is a record of what was sold. The merchant will rename that
-- field, reorder it, change its price or delete it entirely next week,
-- and none of that may rewrite history. This follows order_items, whose
-- product_id and variant_id are deliberately not foreign keys either, and
-- which carries title_snapshot and sku_snapshot for the same reason.
--
-- price_delta is recorded per answer even though the money is already
-- folded into order_items.unit_price. It is there for EXPLANATION, not
-- for arithmetic: "why was this line 27.50 when the shirt is 25.00" has
-- to be answerable years later, and re-deriving it from a catalog that
-- has since changed would produce a plausible lie.
CREATE TABLE order_item_personalisations (
    id            uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id uuid         NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,

    -- Snapshots. field_key is the stable machine key; field_label is what
    -- the buyer actually read when they filled it in.
    field_key   varchar(60)  NOT NULL,
    field_label varchar(120) NOT NULL,
    kind        varchar(20)  NOT NULL,

    -- text/textarea hold the typed value; select holds the chosen
    -- option's LABEL, because that is what the buyer saw and what the
    -- merchant must produce; checkbox holds nothing and relies on the row
    -- existing at all.
    text_value  text,

    price_delta numeric(12,2) NOT NULL DEFAULT 0,

    -- image only. Both keys, because the merchant prints from the
    -- original and the preview is what the buyer approved — see
    -- personalisation_uploads for why conflating them ships blurry
    -- figurines.
    storage_key_original text,
    storage_key          text,
    crop                 jsonb,
    content_type         varchar(100),
    size_bytes           bigint,
    original_filename    varchar(300),

    position   integer     NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT order_item_personalisations_kind_valid
        CHECK (kind IN ('image','text','textarea','select','checkbox')),
    CONSTRAINT order_item_personalisations_price_delta_non_negative
        CHECK (price_delta >= 0)
);

-- The fulfilment read: "everything the buyer supplied for this line".
CREATE INDEX order_item_personalisations_item_idx
    ON order_item_personalisations (order_item_id, position);

-- The erasure read: a subject's artwork is reached through
-- order_items -> orders -> customer_email, and this is the far end of it.
CREATE INDEX order_item_personalisations_image_idx
    ON order_item_personalisations (storage_key_original)
    WHERE storage_key_original IS NOT NULL;

COMMENT ON TABLE order_item_personalisations IS
    'What the buyer supplied, snapshotted onto the order line. No FK to the catalog: an order must survive the merchant editing the field. #967.';
