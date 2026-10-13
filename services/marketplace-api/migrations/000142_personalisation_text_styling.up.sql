-- Migration 142: let a TEXT field be previewed and styled.
--
-- Until now only an image field could carry a mockup and a print area,
-- so a buyer typing "Asha" for a mug never saw it on the mug. This opens
-- the same pair to text and textarea, and adds the two things a merchant
-- needs to offer how the text is produced:
--
--   text_fonts    the font ids the buyer may pick from, in the order the
--                 merchant listed them. Ids from a closed catalogue the
--                 storefront hosts (product.TextFonts), never a free
--                 family name: the preview and the merchant must both be
--                 able to render exactly what was chosen.
--   text_colours  [{"label": "Gold foil", "hex": "#C9A227"}, ...]. Named,
--                 because "Gold foil" is what a merchant produces and a
--                 hex alone is not. Unpriced: a colour that costs more is
--                 a separate checkbox or select, which already carries
--                 money through checkout's reprice.
--
-- Both NULL means "not offered": the buyer types and the merchant uses
-- their house style, exactly as before.
--
-- The order snapshot gains the buyer's choice, copied from the catalog at
-- checkout so a merchant renaming "Gold foil" later does not rewrite what
-- an order asked for.

ALTER TABLE product_personalisation_fields
    DROP CONSTRAINT personalisation_fields_image_only_cols;

ALTER TABLE product_personalisation_fields
    ADD COLUMN text_fonts   text[],
    ADD COLUMN text_colours jsonb,
    -- max_images and min_px remain image-only.
    ADD CONSTRAINT personalisation_fields_image_only_cols
        CHECK (kind = 'image' OR (max_images IS NULL AND min_px IS NULL)),
    -- The mockup pair now serves image, text and textarea.
    ADD CONSTRAINT personalisation_fields_mockup_kinds
        CHECK (
            kind IN ('image','text','textarea')
            OR (mockup_storage_key IS NULL AND print_area IS NULL)
        ),
    ADD CONSTRAINT personalisation_fields_text_style_only_text
        CHECK (
            kind IN ('text','textarea')
            OR (text_fonts IS NULL AND text_colours IS NULL)
        ),
    ADD CONSTRAINT personalisation_fields_text_colours_is_array
        CHECK (text_colours IS NULL OR jsonb_typeof(text_colours) = 'array');

ALTER TABLE order_item_personalisations
    ADD COLUMN text_font         varchar(40),
    ADD COLUMN text_colour_label varchar(60),
    ADD COLUMN text_colour_hex   char(7),
    ADD CONSTRAINT order_item_personalisations_text_colour_hex_format
        CHECK (text_colour_hex IS NULL OR text_colour_hex ~ '^#[0-9A-F]{6}$');
