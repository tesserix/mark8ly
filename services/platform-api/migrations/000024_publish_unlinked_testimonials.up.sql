SET lock_timeout = '5s';
SET statement_timeout = '30s';

ALTER TABLE merchant_testimonials DROP CONSTRAINT IF EXISTS merchant_testimonials_draft_check;
ALTER TABLE merchant_testimonials ADD CONSTRAINT merchant_testimonials_draft_check
    CHECK (store_id IS NOT NULL OR (store_name IS NOT NULL AND char_length(btrim(store_name)) BETWEEN 1 AND 200));
