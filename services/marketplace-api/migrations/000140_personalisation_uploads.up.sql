-- Migration 140: buyer-supplied artwork, before the order exists (#963).
--
-- Owned by a CART, not by a customer. Most buyers are guests, so there is
-- no customer row to hang an upload off; cart_token is the same identity
-- stock_holds already uses (migration 000115), minted by the storefront
-- BFF and carried in the request body.
--
-- TTL is 72 HOURS, not the stock holds' 15 minutes. A shopper who uploads
-- a photo of their child, sleeps on the decision and comes back next
-- evening must not find an empty slot where their photo was. Holds
-- protect inventory from other shoppers; this protects a shopper's own
-- work from us.
--
-- TWO OBJECTS PER UPLOAD, and the distinction is load-bearing:
--
--   storage_key_original  the pristine upload. What the merchant prints
--                         from, and the ONLY thing a print pipeline may
--                         read.
--   storage_key           a browser-cropped, downscaled preview. Cheap to
--                         serve, and lossy — canvas.toBlob() re-encodes,
--                         so a 48 MP phone photo comes out materially
--                         smaller. Mistaking this for the print source is
--                         how a figurine ships with a blurry face.
--   crop                  the rectangle, in ORIGINAL pixel coordinates,
--                         so full resolution can be re-derived later.
--
-- width_px/height_px are CLIENT-SUPPLIED and advisory. They drive only the
-- "this may print soft" warning shown to the buyer. Nothing about money,
-- storage or access depends on them, so there is nothing to gain by
-- lying.
CREATE TABLE personalisation_uploads (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL,
    store_id   uuid        NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    product_id uuid        NOT NULL,
    field_id   uuid        NOT NULL
        REFERENCES product_personalisation_fields(id) ON DELETE CASCADE,

    -- The cart that owns this upload. No FK: stock_holds has no unique
    -- key on cart_token alone, and a cart is not a row anywhere — it is a
    -- token the storefront mints.
    cart_token uuid        NOT NULL,

    storage_key_original text        NOT NULL,
    storage_key          text,
    crop                 jsonb,

    content_hash      varchar(64)  NOT NULL,
    content_type      varchar(100) NOT NULL,
    size_bytes        bigint       NOT NULL,
    original_filename varchar(300) NOT NULL,
    width_px          integer,
    height_px         integer,

    -- pending  : a signed PUT was issued; the object may not exist yet.
    -- verified : the object exists and passed the size/type checks.
    -- claimed  : an order was placed against it. Immune to the sweeper.
    state      varchar(20) NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT personalisation_uploads_state_valid
        CHECK (state IN ('pending','verified','claimed')),
    CONSTRAINT personalisation_uploads_size_positive
        CHECK (size_bytes >= 0),
    CONSTRAINT personalisation_uploads_dims_positive
        CHECK ((width_px IS NULL OR width_px > 0) AND (height_px IS NULL OR height_px > 0))
);

-- Every buyer-facing read is "this upload, for this cart" — the scope that
-- stops one shopper attaching another's photo by id.
CREATE INDEX personalisation_uploads_cart_idx
    ON personalisation_uploads (cart_token);

-- The sweeper's claim query. Partial, because a claimed upload is never
-- swept and is dead weight in this index.
CREATE INDEX personalisation_uploads_expiry_idx
    ON personalisation_uploads (expires_at)
    WHERE state <> 'claimed';

CREATE INDEX personalisation_uploads_tenant_idx
    ON personalisation_uploads (tenant_id);

COMMENT ON TABLE personalisation_uploads IS
    'Buyer-supplied artwork held against a cart until checkout claims it. 72h TTL. #963.';
