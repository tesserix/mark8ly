-- 000142_personalisation_text_styling.down.sql
--
-- Restoring the original image-only constraint fails if any text field
-- now carries a mockup; clear those first so the down migration cannot
-- strand the schema half-reverted.
ALTER TABLE order_item_personalisations
    DROP CONSTRAINT IF EXISTS order_item_personalisations_text_colour_hex_format,
    DROP COLUMN IF EXISTS text_colour_hex,
    DROP COLUMN IF EXISTS text_colour_label,
    DROP COLUMN IF EXISTS text_font;

UPDATE product_personalisation_fields
   SET mockup_storage_key = NULL, print_area = NULL
 WHERE kind <> 'image';

ALTER TABLE product_personalisation_fields
    DROP CONSTRAINT IF EXISTS personalisation_fields_text_colours_is_array,
    DROP CONSTRAINT IF EXISTS personalisation_fields_text_style_only_text,
    DROP CONSTRAINT IF EXISTS personalisation_fields_mockup_kinds,
    DROP CONSTRAINT IF EXISTS personalisation_fields_image_only_cols,
    DROP COLUMN IF EXISTS text_colours,
    DROP COLUMN IF EXISTS text_fonts;

ALTER TABLE product_personalisation_fields
    ADD CONSTRAINT personalisation_fields_image_only_cols
        CHECK (
            (kind = 'image')
            OR (max_images IS NULL AND min_px IS NULL
                AND mockup_storage_key IS NULL AND print_area IS NULL)
        );
