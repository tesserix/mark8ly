SET lock_timeout = '5s';
SET statement_timeout = '30s';

-- Optional person's name on a testimonial card. NULL keeps the card anonymous
-- ("Store owner"); the merchant chooses whether to show it.
ALTER TABLE merchant_testimonials ADD COLUMN display_name TEXT
    CHECK (display_name IS NULL OR char_length(btrim(display_name)) BETWEEN 1 AND 80);
